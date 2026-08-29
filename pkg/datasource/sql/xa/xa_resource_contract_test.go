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

package xa

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/mock"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

func TestXAResourceFactoriesRegisterSupportedDBTypes(t *testing.T) {
	tests := []struct {
		name           string
		dbType         types.DBType
		wantFactory    any
		wantResource   any
		wantClassifier any
	}{
		{
			name:           "mysql",
			dbType:         types.DBTypeMySQL,
			wantFactory:    &mysqlXAResourceFactory{},
			wantResource:   &MysqlXAConn{},
			wantClassifier: &MysqlXAErrorClassifier{},
		},
		{
			name:           "postgres",
			dbType:         types.DBTypePostgreSQL,
			wantFactory:    &postgresXAResourceFactory{},
			wantResource:   &PostgresXAConn{},
			wantClassifier: &PostgresXAErrorClassifier{},
		},
		{
			name:           "mariadb",
			dbType:         types.DBTypeMARIADB,
			wantFactory:    &mariaDBXAResourceFactory{},
			wantResource:   &MariaDBXAConn{},
			wantClassifier: &MariaDBXAErrorClassifier{},
		},
		{
			name:           "oracle",
			dbType:         types.DBTypeOracle,
			wantFactory:    &oracleXAResourceFactory{},
			wantResource:   &OracleXAConn{},
			wantClassifier: &OracleXAErrorClassifier{},
		},
		{
			name:           "dm",
			dbType:         types.DBTypeDM,
			wantFactory:    &dmXAResourceFactory{},
			wantResource:   &DMXAConn{},
			wantClassifier: &DMXAErrorClassifier{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, ok := GetXAResourceFactory(tt.dbType)
			require.True(t, ok)
			assert.IsType(t, tt.wantFactory, factory)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			resource, err := CreateXAResource(mock.NewMockTestDriverConn(ctrl), tt.dbType)
			require.NoError(t, err)
			assert.IsType(t, tt.wantResource, resource)
			assert.IsType(t, tt.wantClassifier, CreateErrorClassifier(tt.dbType))
		})
	}
}

func TestXAResourceFactoriesRejectUnknownDBTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resource, err := CreateXAResource(mock.NewMockTestDriverConn(ctrl), types.DBTypeSQLServer)
	assert.Nil(t, resource)
	assert.ErrorContains(t, err, "no XA resource factory registered")

	assert.IsType(t, &defaultErrorClassifier{}, CreateErrorClassifier(types.DBTypeSQLServer))
}

func TestXAResourceCommonContract(t *testing.T) {
	tests := []struct {
		name                 string
		resource             XAResource
		wantPrepareStatusAPI bool
	}{
		{name: "mysql", resource: &MysqlXAConn{}, wantPrepareStatusAPI: false},
		{name: "postgres", resource: &PostgresXAConn{}, wantPrepareStatusAPI: false},
		{name: "mariadb", resource: &MariaDBXAConn{}, wantPrepareStatusAPI: false},
		{name: "oracle", resource: &OracleXAConn{}, wantPrepareStatusAPI: true},
		{name: "dm", resource: &DMXAConn{}, wantPrepareStatusAPI: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			xids, err := tt.resource.Recover(ctx, TMEndRScan)
			assert.NoError(t, err)
			assert.Nil(t, xids)

			xids, err = tt.resource.Recover(ctx, TMJoin)
			assert.Nil(t, xids)
			assert.ErrorContains(t, err, "invalid arguments")

			assert.Zero(t, tt.resource.GetTransactionTimeout())
			assert.False(t, tt.resource.SetTransactionTimeout(time.Second))
			assert.False(t, tt.resource.IsSameRM(ctx, tt.resource))

			_, ok := tt.resource.(XAResourcePrepareStatus)
			assert.Equal(t, tt.wantPrepareStatusAPI, ok)
		})
	}
}
