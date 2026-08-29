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

package grpc

import (
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/protocol/branch"
	"seata.apache.org/seata-go/v2/pkg/remoting/config"
	remotinggrpc "seata.apache.org/seata-go/v2/pkg/remoting/grpc"
	"seata.apache.org/seata-go/v2/pkg/remoting/grpc/pb"
	"seata.apache.org/seata-go/v2/pkg/rm"
)

func TestGetGrpcRMRemotingInstance(t *testing.T) {
	rm.SetRMRemotingInstance(&GrpcRMRemoting{})
	tests := []struct {
		name     string
		protocol string
		wantType interface{}
	}{
		{
			name:     "GrpcRMRemoting",
			protocol: "grpc",
			wantType: &GrpcRMRemoting{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.InitTransportConfig(&config.TransportConfig{
				Protocol: tt.protocol,
			})

			got := rm.GetRMRemotingInstance()
			assert.NotNil(t, got)
			assert.IsType(t, tt.wantType, got)
		})
	}
}

func TestGrpcRMRemotingBranchRegisterXAType(t *testing.T) {
	var captured *pb.BranchRegisterRequestProto
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggrpc.GetGrpcRemotingClient()), "SendSyncRequest",
		func(_ *remotinggrpc.GrpcRemotingClient, msg interface{}) (interface{}, error) {
			captured = msg.(*pb.BranchRegisterRequestProto)
			return &pb.BranchRegisterResponseProto{
				AbstractTransactionResponse: &pb.AbstractTransactionResponseProto{
					AbstractResultMessage: &pb.AbstractResultMessageProto{
						ResultCode: pb.ResultCodeProto_Success,
					},
				},
				BranchId: 1001,
			}, nil
		})
	defer patches.Reset()

	branchID, err := (&GrpcRMRemoting{}).BranchRegister(rm.BranchRegisterParam{
		BranchType:      branch.BranchTypeXA,
		Xid:             "xa-register-xid",
		ResourceId:      "xa-resource",
		LockKeys:        "xa-lock",
		ApplicationData: "xa-register",
	})

	assert.NoError(t, err)
	assert.EqualValues(t, 1001, branchID)
	if assert.NotNil(t, captured) {
		assert.Equal(t, pb.MessageTypeProto_TYPE_BRANCH_REGISTER, captured.AbstractTransactionRequest.AbstractMessage.MessageType)
		assert.Equal(t, "xa-register-xid", captured.Xid)
		assert.Equal(t, "xa-resource", captured.ResourceId)
		assert.Equal(t, "xa-lock", captured.LockKey)
		assert.Equal(t, pb.BranchTypeProto_XA, captured.BranchType)
		assert.EqualValues(t, branch.BranchTypeXA, pb.BranchTypeProto_XA)
		assert.Equal(t, "xa-register", captured.ApplicationData)
	}
}

func TestGrpcRMRemotingBranchReportReadonlyStatus(t *testing.T) {
	var captured *pb.BranchReportRequestProto
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggrpc.GetGrpcRemotingClient()), "SendSyncRequest",
		func(_ *remotinggrpc.GrpcRemotingClient, msg interface{}) (interface{}, error) {
			captured = msg.(*pb.BranchReportRequestProto)
			return &pb.BranchReportResponseProto{
				AbstractTransactionResponse: &pb.AbstractTransactionResponseProto{
					AbstractResultMessage: &pb.AbstractResultMessageProto{
						ResultCode: pb.ResultCodeProto_Success,
					},
				},
			}, nil
		})
	defer patches.Reset()

	err := (&GrpcRMRemoting{}).BranchReport(rm.BranchReportParam{
		BranchType:      branch.BranchTypeXA,
		Xid:             "readonly-xid",
		BranchId:        13,
		Status:          branch.BranchStatusPhaseoneReadonly,
		ApplicationData: "readonly",
	})

	assert.NoError(t, err)
	if assert.NotNil(t, captured) {
		assert.Equal(t, pb.MessageTypeProto_TYPE_BRANCH_STATUS_REPORT, captured.AbstractTransactionRequest.AbstractMessage.MessageType)
		assert.Equal(t, "readonly-xid", captured.Xid)
		assert.EqualValues(t, 13, captured.BranchId)
		assert.Equal(t, pb.BranchStatusProto_PhaseOne_RDONLY, captured.Status)
		assert.Equal(t, pb.BranchTypeProto_XA, captured.BranchType)
		assert.EqualValues(t, branch.BranchTypeXA, pb.BranchTypeProto_XA)
		assert.Equal(t, "XA", pb.BranchTypeProto_XA.String())
		assert.Equal(t, "readonly", captured.ApplicationData)
	}
}

func TestGrpcRMRemotingLockQueryXAType(t *testing.T) {
	var captured *pb.GlobalLockQueryRequestProto
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggrpc.GetGrpcRemotingClient()), "SendSyncRequest",
		func(_ *remotinggrpc.GrpcRemotingClient, msg interface{}) (interface{}, error) {
			captured = msg.(*pb.GlobalLockQueryRequestProto)
			return &pb.GlobalLockQueryResponseProto{
				Lockable: true,
			}, nil
		})
	defer patches.Reset()

	lockable, err := (&GrpcRMRemoting{}).LockQuery(rm.LockQueryParam{
		BranchType: branch.BranchTypeXA,
		Xid:        "lock-query-xid",
		ResourceId: "xa-lock-resource",
		LockKeys:   "table:1",
	})

	assert.NoError(t, err)
	assert.True(t, lockable)
	if assert.NotNil(t, captured) {
		registerRequest := captured.BranchRegisterRequest
		if assert.NotNil(t, registerRequest) {
			assert.Equal(t, pb.MessageTypeProto_TYPE_GLOBAL_LOCK_QUERY, registerRequest.AbstractTransactionRequest.AbstractMessage.MessageType)
			assert.Equal(t, "lock-query-xid", registerRequest.Xid)
			assert.Equal(t, "xa-lock-resource", registerRequest.ResourceId)
			assert.Equal(t, "table:1", registerRequest.LockKey)
			assert.Equal(t, pb.BranchTypeProto_XA, registerRequest.BranchType)
			assert.EqualValues(t, branch.BranchTypeXA, pb.BranchTypeProto_XA)
		}
	}
}
