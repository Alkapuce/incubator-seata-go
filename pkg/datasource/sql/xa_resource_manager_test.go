/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sql

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/xa"
	"seata.apache.org/seata-go/v2/pkg/protocol/branch"
	"seata.apache.org/seata-go/v2/pkg/protocol/message"
	"seata.apache.org/seata-go/v2/pkg/remoting/getty"
	"seata.apache.org/seata-go/v2/pkg/rm"
	gettyrm "seata.apache.org/seata-go/v2/pkg/rm/remoting/getty"
)

type xaResourceManagerTestConn struct{}

func (c *xaResourceManagerTestConn) Prepare(query string) (driver.Stmt, error) { return nil, nil }
func (c *xaResourceManagerTestConn) Close() error                              { return nil }
func (c *xaResourceManagerTestConn) Begin() (driver.Tx, error)                 { return nil, nil }

type xaResourceManagerTestXAResource struct {
	commitCnt   int32
	rollbackCnt int32
	commitXID   string
	rollbackXID string
}

func (r *xaResourceManagerTestXAResource) Commit(ctx context.Context, xid string, onePhase bool) error {
	atomic.AddInt32(&r.commitCnt, 1)
	r.commitXID = xid
	return nil
}

func (r *xaResourceManagerTestXAResource) End(ctx context.Context, xid string, flags int) error {
	return nil
}

func (r *xaResourceManagerTestXAResource) Forget(ctx context.Context, xid string) error {
	return nil
}

func (r *xaResourceManagerTestXAResource) GetTransactionTimeout() time.Duration {
	return 0
}

func (r *xaResourceManagerTestXAResource) IsSameRM(ctx context.Context, resource xa.XAResource) bool {
	return r == resource
}

func (r *xaResourceManagerTestXAResource) XAPrepare(ctx context.Context, xid string) error {
	return nil
}

func (r *xaResourceManagerTestXAResource) Recover(ctx context.Context, flag int) ([]string, error) {
	return nil, nil
}

func (r *xaResourceManagerTestXAResource) Rollback(ctx context.Context, xid string) error {
	atomic.AddInt32(&r.rollbackCnt, 1)
	r.rollbackXID = xid
	return nil
}

func (r *xaResourceManagerTestXAResource) SetTransactionTimeout(duration time.Duration) bool {
	return true
}

func (r *xaResourceManagerTestXAResource) Start(ctx context.Context, xid string, flags int) error {
	return nil
}

func TestXAResourceManager_LockQuery(t *testing.T) {
	tests := []struct {
		name    string
		resp    interface{}
		respErr error
		want    bool
		wantErr string
	}{
		{
			name: "lockable",
			resp: message.GlobalLockQueryResponse{Lockable: true},
			want: true,
		},
		{
			name: "unlockable",
			resp: message.GlobalLockQueryResponse{Lockable: false},
			want: false,
		},
		{
			name:    "remoting error",
			respErr: errors.New("network timeout"),
			want:    false,
			wantErr: "network timeout",
		},
	}

	param := rm.LockQueryParam{
		BranchType: branch.BranchTypeXA,
		ResourceId: "jdbc:mysql://test/resource",
		Xid:        "test-xid",
		LockKeys:   "user:1",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(getty.GetGettyRemotingClient()), "SendSyncRequest",
				func(_ *getty.GettyRemotingClient, msg interface{}) (interface{}, error) {
					req, ok := msg.(message.GlobalLockQueryRequest)
					if assert.True(t, ok) {
						assert.Equal(t, param.BranchType, req.BranchType)
						assert.Equal(t, param.ResourceId, req.ResourceId)
						assert.Equal(t, param.Xid, req.Xid)
						assert.Equal(t, param.LockKeys, req.LockKey)
					}
					return tt.resp, tt.respErr
				})
			defer patches.Reset()

			xaManager := &XAResourceManager{rmRemoting: &gettyrm.GettyRMRemoting{}}

			got, err := xaManager.LockQuery(context.Background(), param)

			assert.Equal(t, tt.want, got)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}

func TestXAResourceManager_BranchCommitUsesHeldConnectionAndReleases(t *testing.T) {
	tests := []struct {
		name   string
		dbType types.DBType
	}{
		{name: "mariadb", dbType: types.DBTypeMARIADB},
		{name: "oracle", dbType: types.DBTypeOracle},
		{name: "dm", dbType: types.DBTypeDM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xaID := XaIdBuild("global-branch-commit", 3001)
			resourceID := "jdbc:" + tt.name + "://test/resource"
			testXAResource := &xaResourceManagerTestXAResource{}
			dbResource := &DBResource{
				resourceID:   resourceID,
				dbType:       tt.dbType,
				shouldBeHeld: true,
			}
			xaConn := &XAConn{
				Conn: &Conn{
					targetConn: &xaResourceManagerTestConn{},
					res:        dbResource,
				},
				xaBranchXid:       xaID,
				xaResource:        testXAResource,
				xaErrorClassifier: xa.CreateErrorClassifier(tt.dbType),
				isConnKept:        true,
			}
			assert.NoError(t, dbResource.Hold(xaID.String(), xaConn))

			xaManager := &XAResourceManager{}
			xaManager.resourceCache.Store(resourceID, dbResource)

			status, err := xaManager.BranchCommit(context.Background(), rm.BranchResource{
				BranchType: branch.BranchTypeXA,
				Xid:        xaID.GetGlobalXid(),
				BranchId:   int64(xaID.GetBranchId()),
				ResourceId: resourceID,
			})

			assert.NoError(t, err)
			assert.EqualValues(t, branch.BranchStatusPhasetwoCommitted, status)
			assert.Equal(t, int32(1), atomic.LoadInt32(&testXAResource.commitCnt))
			assert.Equal(t, xaID.String(), testXAResource.commitXID)
			_, ok := dbResource.Lookup(xaID.String())
			assert.False(t, ok, "branch commit must release the held XA connection")
		})
	}
}

func TestXAResourceManager_BranchRollbackUsesHeldConnectionAndReleases(t *testing.T) {
	tests := []struct {
		name   string
		dbType types.DBType
	}{
		{name: "mariadb", dbType: types.DBTypeMARIADB},
		{name: "oracle", dbType: types.DBTypeOracle},
		{name: "dm", dbType: types.DBTypeDM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xaID := XaIdBuild("global-branch-rollback", 4001)
			resourceID := "jdbc:" + tt.name + "://test/resource"
			testXAResource := &xaResourceManagerTestXAResource{}
			dbResource := &DBResource{
				resourceID:   resourceID,
				dbType:       tt.dbType,
				shouldBeHeld: true,
			}
			xaConn := &XAConn{
				Conn: &Conn{
					targetConn: &xaResourceManagerTestConn{},
					res:        dbResource,
				},
				xaBranchXid:       xaID,
				xaResource:        testXAResource,
				xaErrorClassifier: xa.CreateErrorClassifier(tt.dbType),
				isConnKept:        true,
			}
			assert.NoError(t, dbResource.Hold(xaID.String(), xaConn))

			xaManager := &XAResourceManager{}
			xaManager.resourceCache.Store(resourceID, dbResource)

			status, err := xaManager.BranchRollback(context.Background(), rm.BranchResource{
				BranchType: branch.BranchTypeXA,
				Xid:        xaID.GetGlobalXid(),
				BranchId:   int64(xaID.GetBranchId()),
				ResourceId: resourceID,
			})

			assert.NoError(t, err)
			assert.EqualValues(t, branch.BranchStatusPhasetwoRollbacked, status)
			assert.Equal(t, int32(1), atomic.LoadInt32(&testXAResource.rollbackCnt))
			assert.Equal(t, xaID.String(), testXAResource.rollbackXID)
			_, ok := dbResource.Lookup(xaID.String())
			assert.False(t, ok, "branch rollback must release the held XA connection")
		})
	}
}
