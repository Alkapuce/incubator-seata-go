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
	"github.com/bluele/gcache"
	"github.com/go-sql-driver/mysql"
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
	commitErr   error
	rollbackErr error
}

func (r *xaResourceManagerTestXAResource) Commit(ctx context.Context, xid string, onePhase bool) error {
	atomic.AddInt32(&r.commitCnt, 1)
	r.commitXID = xid
	return r.commitErr
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
	return r.rollbackErr
}

func (r *xaResourceManagerTestXAResource) SetTransactionTimeout(duration time.Duration) bool {
	return true
}

func (r *xaResourceManagerTestXAResource) Start(ctx context.Context, xid string, flags int) error {
	return nil
}

func resetXAResourceManagerBranchStatusCache(t *testing.T) {
	t.Helper()
	branchStatusCache = gcache.New(1024).LRU().Expiration(time.Minute * 10).Build()
	t.Cleanup(func() {
		branchStatusCache = nil
	})
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

func TestXAResourceManager_BranchCommitFailureStatus(t *testing.T) {
	tests := []struct {
		name       string
		dbType     types.DBType
		commitErr  error
		wantCached branch.BranchStatus
	}{
		{
			name:       "mariadb already ended",
			dbType:     types.DBTypeMARIADB,
			commitErr:  &mysql.MySQLError{Number: types.ErrCodeXAER_RMFAIL_IDLE, Message: "XAER_RMFAIL: The command cannot be executed when global transaction is in the PREPARED state"},
			wantCached: branch.BranchStatusPhasetwoCommitted,
		},
		{
			name:       "oracle xaer nota",
			dbType:     types.DBTypeOracle,
			commitErr:  errors.New("ORA-20777: DBMS_XA.XA_COMMIT failed with code -4, oracle error 0"),
			wantCached: branch.BranchStatusPhasetwoCommitted,
		},
		{
			name:       "dm xaer nota",
			dbType:     types.DBTypeDM,
			commitErr:  errors.New("DM DBMS_XA.XA_COMMIT failed with code XAER_NOTA"),
			wantCached: branch.BranchStatusPhasetwoCommitted,
		},
		{
			name:       "generic commit error",
			dbType:     types.DBTypeMARIADB,
			commitErr:  errors.New("driver commit timeout"),
			wantCached: branch.BranchStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetXAResourceManagerBranchStatusCache(t)
			xaID := XaIdBuild("global-branch-commit-failure", 5001)
			resourceID := "jdbc:" + tt.name + "://test/resource"
			testXAResource := &xaResourceManagerTestXAResource{commitErr: tt.commitErr}
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

			assert.ErrorIs(t, err, tt.commitErr)
			assert.EqualValues(t, branch.BranchStatusPhasetwoCommitFailedRetryable, status)
			assert.Equal(t, int32(1), atomic.LoadInt32(&testXAResource.commitCnt))
			cached, cacheErr := branchStatus(xaID.String())
			assert.NoError(t, cacheErr)
			assert.EqualValues(t, tt.wantCached, cached)
			_, ok := dbResource.Lookup(xaID.String())
			assert.False(t, ok, "branch commit failure must release the held XA connection")
		})
	}
}

func TestXAResourceManager_BranchRollbackFailureStatus(t *testing.T) {
	tests := []struct {
		name        string
		dbType      types.DBType
		rollbackErr error
		wantCached  branch.BranchStatus
	}{
		{
			name:        "mariadb already ended",
			dbType:      types.DBTypeMARIADB,
			rollbackErr: &mysql.MySQLError{Number: types.ErrCodeXAER_RMFAIL_IDLE, Message: "XAER_RMFAIL: The command cannot be executed when global transaction is in the IDLE state"},
			wantCached:  branch.BranchStatusPhasetwoRollbacked,
		},
		{
			name:        "oracle xaer nota",
			dbType:      types.DBTypeOracle,
			rollbackErr: errors.New("ORA-20777: DBMS_XA.XA_ROLLBACK failed with code -3, oracle error 24756"),
			wantCached:  branch.BranchStatusPhasetwoRollbacked,
		},
		{
			name:        "dm xaer nota",
			dbType:      types.DBTypeDM,
			rollbackErr: errors.New("DM DBMS_XA.XA_ROLLBACK failed with code XAER_NOTA"),
			wantCached:  branch.BranchStatusPhasetwoRollbacked,
		},
		{
			name:        "generic rollback error",
			dbType:      types.DBTypeMARIADB,
			rollbackErr: errors.New("driver rollback timeout"),
			wantCached:  branch.BranchStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetXAResourceManagerBranchStatusCache(t)
			xaID := XaIdBuild("global-branch-rollback-failure", 6001)
			resourceID := "jdbc:" + tt.name + "://test/resource"
			testXAResource := &xaResourceManagerTestXAResource{rollbackErr: tt.rollbackErr}
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

			assert.ErrorIs(t, err, tt.rollbackErr)
			assert.EqualValues(t, branch.BranchStatusPhasetwoRollbackFailedRetryable, status)
			assert.Equal(t, int32(1), atomic.LoadInt32(&testXAResource.rollbackCnt))
			cached, cacheErr := branchStatus(xaID.String())
			assert.NoError(t, cacheErr)
			assert.EqualValues(t, tt.wantCached, cached)
			_, ok := dbResource.Lookup(xaID.String())
			assert.False(t, ok, "branch rollback failure must release the held XA connection")
		})
	}
}

func TestXAResourceManager_BranchCommitUnknownResourceStatus(t *testing.T) {
	xaID := XaIdBuild("global-branch-commit-unknown-resource", 7001)
	xaManager := &XAResourceManager{}

	status, err := xaManager.BranchCommit(context.Background(), rm.BranchResource{
		BranchType: branch.BranchTypeXA,
		Xid:        xaID.GetGlobalXid(),
		BranchId:   int64(xaID.GetBranchId()),
		ResourceId: "jdbc:missing://test/resource",
	})

	assert.Error(t, err)
	assert.EqualValues(t, branch.BranchStatusPhasetwoCommitFailedUnretryable, status)
}

func TestXAResourceManager_BranchRollbackUnknownResourceStatus(t *testing.T) {
	xaID := XaIdBuild("global-branch-rollback-unknown-resource", 8001)
	xaManager := &XAResourceManager{}

	status, err := xaManager.BranchRollback(context.Background(), rm.BranchResource{
		BranchType: branch.BranchTypeXA,
		Xid:        xaID.GetGlobalXid(),
		BranchId:   int64(xaID.GetBranchId()),
		ResourceId: "jdbc:missing://test/resource",
	})

	assert.Error(t, err)
	assert.EqualValues(t, branch.BranchStatusPhasetwoRollbackFailedUnretryable, status)
}
