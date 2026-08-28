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

package client

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/protocol"
	"seata.apache.org/seata-go/v2/pkg/protocol/branch"
	"seata.apache.org/seata-go/v2/pkg/protocol/message"
	"seata.apache.org/seata-go/v2/pkg/remoting/config"
	remotinggrpc "seata.apache.org/seata-go/v2/pkg/remoting/grpc"
	"seata.apache.org/seata-go/v2/pkg/remoting/grpc/pb"
	"seata.apache.org/seata-go/v2/pkg/rm"
)

type xaProcessorTestRM struct {
	commitResource   rm.BranchResource
	rollbackResource rm.BranchResource
}

func (m *xaProcessorTestRM) BranchCommit(ctx context.Context, resource rm.BranchResource) (branch.BranchStatus, error) {
	m.commitResource = resource
	return branch.BranchStatusPhasetwoCommitted, nil
}

func (m *xaProcessorTestRM) BranchRollback(ctx context.Context, resource rm.BranchResource) (branch.BranchStatus, error) {
	m.rollbackResource = resource
	return branch.BranchStatusPhasetwoRollbacked, nil
}

func (m *xaProcessorTestRM) BranchRegister(ctx context.Context, param rm.BranchRegisterParam) (int64, error) {
	return 0, nil
}

func (m *xaProcessorTestRM) BranchReport(ctx context.Context, param rm.BranchReportParam) error {
	return nil
}

func (m *xaProcessorTestRM) LockQuery(ctx context.Context, param rm.LockQueryParam) (bool, error) {
	return false, nil
}

func (m *xaProcessorTestRM) RegisterResource(resource rm.Resource) error {
	return nil
}

func (m *xaProcessorTestRM) UnregisterResource(resource rm.Resource) error {
	return nil
}

func (m *xaProcessorTestRM) GetCachedResources() *sync.Map {
	return &sync.Map{}
}

func (m *xaProcessorTestRM) GetBranchType() branch.BranchType {
	return branch.BranchTypeXA
}

func TestGrpcBranchProcessorsRouteXAType(t *testing.T) {
	config.InitTransportConfig(&config.TransportConfig{Protocol: protocol.ProtocolGRPC.String()})
	testRM := &xaProcessorTestRM{}
	rm.GetRmCacheInstance().RegisterResourceManager(testRM)
	defer rm.GetRmCacheInstance().UnregisterResourceManager(branch.BranchTypeXA)

	responses := make(map[int32]interface{})
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggrpc.GetGrpcRemotingClient()), "SendAsyncResponse",
		func(_ *remotinggrpc.GrpcRemotingClient, msgID int32, msg interface{}) error {
			responses[msgID] = msg
			return nil
		})
	defer patches.Reset()

	commitRequest := message.RpcMessage{
		ID: 301,
		Body: &pb.BranchCommitRequestProto{
			AbstractBranchEndRequest: &pb.AbstractBranchEndRequestProto{
				Xid:             "xa-xid",
				BranchId:        1001,
				BranchType:      pb.BranchTypeProto_XA,
				ResourceId:      "xa-resource",
				ApplicationData: "xa-commit",
			},
		},
	}
	err := (&rmBranchCommitProcessor{}).handleGrpcBranchCommit(context.Background(), commitRequest)
	assert.NoError(t, err)
	assert.Equal(t, branch.BranchTypeXA, testRM.commitResource.BranchType)
	assert.Equal(t, "xa-xid", testRM.commitResource.Xid)
	assert.EqualValues(t, 1001, testRM.commitResource.BranchId)
	assert.Equal(t, "xa-resource", testRM.commitResource.ResourceId)
	assert.Equal(t, []byte("xa-commit"), testRM.commitResource.ApplicationData)

	commitResponse, ok := responses[301].(*pb.BranchCommitResponseProto)
	if assert.True(t, ok) {
		assert.Equal(t, pb.BranchStatusProto_PhaseTwo_Committed, commitResponse.AbstractBranchEndResponse.BranchStatus)
	}

	rollbackRequest := message.RpcMessage{
		ID: 302,
		Body: &pb.BranchRollbackRequestProto{
			AbstractBranchEndRequest: &pb.AbstractBranchEndRequestProto{
				Xid:             "xa-xid",
				BranchId:        1002,
				BranchType:      pb.BranchTypeProto_XA,
				ResourceId:      "xa-resource",
				ApplicationData: "xa-rollback",
			},
		},
	}
	err = (&rmBranchRollbackProcessor{}).handleGrpcBranchRollback(context.Background(), rollbackRequest)
	assert.NoError(t, err)
	assert.Equal(t, branch.BranchTypeXA, testRM.rollbackResource.BranchType)
	assert.Equal(t, "xa-xid", testRM.rollbackResource.Xid)
	assert.EqualValues(t, 1002, testRM.rollbackResource.BranchId)
	assert.Equal(t, "xa-resource", testRM.rollbackResource.ResourceId)
	assert.Equal(t, []byte("xa-rollback"), testRM.rollbackResource.ApplicationData)

	rollbackResponse, ok := responses[302].(*pb.BranchRollbackResponseProto)
	if assert.True(t, ok) {
		assert.Equal(t, pb.BranchStatusProto_PhaseTwo_Rollbacked, rollbackResponse.AbstractBranchEndResponse.BranchStatus)
	}
}
