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
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
	"time"

	"seata.apache.org/seata-go/v2/pkg/rm"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/mock"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
	"seata.apache.org/seata-go/v2/pkg/protocol/branch"
	"seata.apache.org/seata-go/v2/pkg/util/reflectx"
)

func initMockResourceManager(branchType branch.BranchType, ctrl *gomock.Controller) *mock.MockDataSourceManager {
	mockResourceMgr := mock.NewMockDataSourceManager(ctrl)
	mockResourceMgr.SetBranchType(branchType)
	mockResourceMgr.EXPECT().BranchRegister(gomock.Any(), gomock.Any()).AnyTimes().Return(int64(0), nil)
	rm.GetRmCacheInstance().RegisterResourceManager(mockResourceMgr)
	mockResourceMgr.EXPECT().RegisterResource(gomock.Any()).AnyTimes().Return(nil)
	mockResourceMgr.EXPECT().CreateTableMetaCache(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)

	return mockResourceMgr
}

func Test_seataATDriver_Open(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMgr := initMockResourceManager(branch.BranchTypeAT, ctrl)
	_ = mockMgr

	db, err := sql.Open("seata-at-mysql", "root:seata_go@tcp(127.0.0.1:3306)/seata_go_test?multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	_ = initMockAtConnector(t, ctrl, db, func(t *testing.T, ctrl *gomock.Controller) driver.Connector {
		connector := mock.NewMockTestDriverConnector(ctrl)
		connector.EXPECT().Connect(gomock.Any()).Return(nil, fmt.Errorf("connect error"))
		return connector
	})

	conn, err := db.Conn(context.Background())
	assert.NotNil(t, err)
	assert.Nil(t, conn)
}

func Test_seataATDriver_OpenConnector(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMgr := initMockResourceManager(branch.BranchTypeAT, ctrl)
	_ = mockMgr

	db, err := sql.Open("seata-at-mysql", "root:seata_go@tcp(127.0.0.1:3306)/seata_go_test?multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	v := reflect.ValueOf(db)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	field := v.FieldByName("connector")
	fieldVal := reflectx.GetUnexportedField(field)

	_, ok := fieldVal.(*seataATConnector)
	assert.True(t, ok, "need return seata at connector")
}

func Test_seataATPostgresDriver_OpenConnector(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMgr := initMockResourceManager(branch.BranchTypeAT, ctrl)
	_ = mockMgr

	db, err := sql.Open(SeataATPostgresDriver, postgresTestDSN)
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	v := reflect.ValueOf(db)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	field := v.FieldByName("connector")
	fieldVal := reflectx.GetUnexportedField(field)

	connector, ok := fieldVal.(*seataATConnector)
	assert.True(t, ok, "need return seata at connector")
	assert.Equal(t, types.DBTypePostgreSQL, connector.dbType)
	assert.Equal(t, "seata_go_test", connector.dbName)
}

func Test_seataXADriver_OpenConnector(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMgr := initMockResourceManager(branch.BranchTypeXA, ctrl)
	_ = mockMgr

	db, err := sql.Open("seata-xa-mysql", "root:seata_go@tcp(127.0.0.1:3306)/seata_go_test?multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	v := reflect.ValueOf(db)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	field := v.FieldByName("connector")
	fieldVal := reflectx.GetUnexportedField(field)

	_, ok := fieldVal.(*seataXAConnector)
	assert.True(t, ok, "need return seata xa connector")
}

func TestRegisterSeataXADriver(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMgr := initMockResourceManager(branch.BranchTypeXA, ctrl)
	_ = mockMgr

	mockDriver := mock.NewMockTestDriver(ctrl)
	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockDriver.EXPECT().Open("opaque-dsn").AnyTimes().Return(mockConn, nil)
	mockConn.EXPECT().Close().AnyTimes().Return(nil)

	driverName := fmt.Sprintf("seata-xa-vendor-%d", time.Now().UnixNano())
	err := RegisterSeataXADriver(driverName, SeataDriverDescriptor{
		DBType:     types.DBTypeOracle,
		Target:     mockDriver,
		TargetName: "vendor-oracle",
		ParseDBName: func(dsn string) (string, error) {
			assert.Equal(t, "opaque-dsn", dsn)
			return "vendor_db", nil
		},
	})
	assert.NoError(t, err)

	db, err := sql.Open(driverName, "opaque-dsn")
	assert.NoError(t, err)
	defer db.Close()

	v := reflect.ValueOf(db)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	field := v.FieldByName("connector")
	fieldVal := reflectx.GetUnexportedField(field)
	connector, ok := fieldVal.(*seataXAConnector)
	assert.True(t, ok, "need return seata xa connector")
	assert.Equal(t, types.DBTypeOracle, connector.dbType)
	assert.Equal(t, "vendor_db", connector.dbName)
	assert.Equal(t, "vendor-oracle", connector.targetName)

	conn, err := connector.Connect(context.Background())
	assert.NoError(t, err)
	xaConn, ok := conn.(*XAConn)
	assert.True(t, ok, "need return seata xa connection")
	assert.Equal(t, types.DBTypeOracle, xaConn.dbType)
	assert.Equal(t, "vendor_db", xaConn.dbName)
}

func TestRegisterSeataXADriverRejectsInvalidDescriptor(t *testing.T) {
	assert.Error(t, RegisterSeataXADriver("", SeataDriverDescriptor{}))
	assert.Error(t, RegisterSeataXADriver("seata-xa-invalid-missing-driver", SeataDriverDescriptor{
		DBType:      types.DBTypeOracle,
		ParseDBName: func(string) (string, error) { return "db", nil },
	}))
	assert.Error(t, RegisterSeataXADriver("seata-xa-invalid-missing-parser", SeataDriverDescriptor{
		DBType: types.DBTypeOracle,
		Target: mock.NewMockTestDriver(gomock.NewController(t)),
	}))
}

func TestRegisterSeataXADriverUsesDBTypeAsDefaultTargetName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMgr := initMockResourceManager(branch.BranchTypeXA, ctrl)
	_ = mockMgr

	mockDriver := mock.NewMockTestDriver(ctrl)
	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockDriver.EXPECT().Open("opaque-dsn").AnyTimes().Return(mockConn, nil)
	mockConn.EXPECT().Close().AnyTimes().Return(nil)

	driverName := fmt.Sprintf("seata-xa-vendor-default-target-%d", time.Now().UnixNano())
	err := RegisterSeataXADriver(driverName, SeataDriverDescriptor{
		DBType: types.DBTypeOracle,
		Target: mockDriver,
		ParseDBName: func(string) (string, error) {
			return "vendor_db", nil
		},
	})
	assert.NoError(t, err)

	db, err := sql.Open(driverName, "opaque-dsn")
	assert.NoError(t, err)
	defer db.Close()

	v := reflect.ValueOf(db)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	field := v.FieldByName("connector")
	fieldVal := reflectx.GetUnexportedField(field)
	connector, ok := fieldVal.(*seataXAConnector)
	assert.True(t, ok, "need return seata xa connector")
	assert.Equal(t, types.DBTypeOracle.String(), connector.targetName)
}

func TestRegisterSeataXADriverReturnsDuplicateRegistrationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	driverName := fmt.Sprintf("seata-xa-vendor-duplicate-%d", time.Now().UnixNano())
	descriptor := SeataDriverDescriptor{
		DBType: types.DBTypeOracle,
		Target: mock.NewMockTestDriver(ctrl),
		ParseDBName: func(string) (string, error) {
			return "vendor_db", nil
		},
	}

	assert.NoError(t, RegisterSeataXADriver(driverName, descriptor))
	err := RegisterSeataXADriver(driverName, descriptor)
	assert.ErrorContains(t, err, "register seata xa driver")
	assert.ErrorContains(t, err, driverName)
}
