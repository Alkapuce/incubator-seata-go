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

package getty

import (
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/protocol/branch"
	"seata.apache.org/seata-go/v2/pkg/protocol/message"
	"seata.apache.org/seata-go/v2/pkg/remoting/config"
	remotinggetty "seata.apache.org/seata-go/v2/pkg/remoting/getty"
	"seata.apache.org/seata-go/v2/pkg/rm"
)

func TestGetGettyRMRemotingInstance(t *testing.T) {
	rm.SetRMRemotingInstance(&GettyRMRemoting{})
	tests := []struct {
		name     string
		protocol string
		wantType interface{}
	}{
		{
			name:     "GettyRMRemoting",
			protocol: "seata",
			wantType: &GettyRMRemoting{},
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

func TestGettyRMRemotingBranchRegisterXAType(t *testing.T) {
	var captured message.BranchRegisterRequest
	capturedOK := false
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggetty.GetGettyRemotingClient()), "SendSyncRequest",
		func(_ *remotinggetty.GettyRemotingClient, msg interface{}) (interface{}, error) {
			req, ok := msg.(message.BranchRegisterRequest)
			if assert.True(t, ok) {
				captured = req
				capturedOK = true
			}
			return message.BranchRegisterResponse{
				AbstractTransactionResponse: message.AbstractTransactionResponse{
					AbstractResultMessage: message.AbstractResultMessage{
						ResultCode: message.ResultCodeSuccess,
					},
				},
				BranchId: 1001,
			}, nil
		})
	defer patches.Reset()

	branchID, err := (&GettyRMRemoting{}).BranchRegister(rm.BranchRegisterParam{
		BranchType:      branch.BranchTypeXA,
		Xid:             "xa-register-xid",
		ResourceId:      "xa-resource",
		LockKeys:        "xa-lock",
		ApplicationData: "xa-register",
	})

	assert.NoError(t, err)
	assert.EqualValues(t, 1001, branchID)
	if assert.True(t, capturedOK) {
		assert.Equal(t, message.MessageTypeBranchRegister, captured.GetTypeCode())
		assert.Equal(t, "xa-register-xid", captured.Xid)
		assert.Equal(t, "xa-resource", captured.ResourceId)
		assert.Equal(t, "xa-lock", captured.LockKey)
		assert.Equal(t, branch.BranchTypeXA, captured.BranchType)
		assert.Equal(t, []byte("xa-register"), captured.ApplicationData)
	}
}

func TestGettyRMRemotingBranchReportReadonlyStatus(t *testing.T) {
	var captured message.BranchReportRequest
	capturedOK := false
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggetty.GetGettyRemotingClient()), "SendSyncRequest",
		func(_ *remotinggetty.GettyRemotingClient, msg interface{}) (interface{}, error) {
			req, ok := msg.(message.BranchReportRequest)
			if assert.True(t, ok) {
				captured = req
				capturedOK = true
			}
			return message.BranchReportResponse{
				AbstractTransactionResponse: message.AbstractTransactionResponse{
					AbstractResultMessage: message.AbstractResultMessage{
						ResultCode: message.ResultCodeSuccess,
					},
				},
			}, nil
		})
	defer patches.Reset()

	err := (&GettyRMRemoting{}).BranchReport(rm.BranchReportParam{
		BranchType:      branch.BranchTypeXA,
		Xid:             "readonly-xid",
		BranchId:        13,
		Status:          branch.BranchStatusPhaseoneReadonly,
		ApplicationData: "readonly",
	})

	assert.NoError(t, err)
	if assert.True(t, capturedOK) {
		assert.Equal(t, message.MessageTypeBranchStatusReport, captured.GetTypeCode())
		assert.Equal(t, "readonly-xid", captured.Xid)
		assert.EqualValues(t, 13, captured.BranchId)
		assert.Equal(t, branch.BranchStatusPhaseoneReadonly, captured.Status)
		assert.Equal(t, branch.BranchTypeXA, captured.BranchType)
		assert.Equal(t, []byte("readonly"), captured.ApplicationData)
	}
}

func TestGettyRMRemotingLockQueryXAType(t *testing.T) {
	var captured message.GlobalLockQueryRequest
	capturedOK := false
	patches := gomonkey.ApplyMethod(reflect.TypeOf(remotinggetty.GetGettyRemotingClient()), "SendSyncRequest",
		func(_ *remotinggetty.GettyRemotingClient, msg interface{}) (interface{}, error) {
			req, ok := msg.(message.GlobalLockQueryRequest)
			if assert.True(t, ok) {
				captured = req
				capturedOK = true
			}
			return message.GlobalLockQueryResponse{Lockable: true}, nil
		})
	defer patches.Reset()

	lockable, err := (&GettyRMRemoting{}).LockQuery(rm.LockQueryParam{
		BranchType: branch.BranchTypeXA,
		Xid:        "lock-query-xid",
		ResourceId: "xa-lock-resource",
		LockKeys:   "table:1",
	})

	assert.NoError(t, err)
	assert.True(t, lockable)
	if assert.True(t, capturedOK) {
		assert.Equal(t, message.MessageTypeGlobalLockQuery, captured.GetTypeCode())
		assert.Equal(t, "lock-query-xid", captured.Xid)
		assert.Equal(t, "xa-lock-resource", captured.ResourceId)
		assert.Equal(t, "table:1", captured.LockKey)
		assert.Equal(t, branch.BranchTypeXA, captured.BranchType)
	}
}
