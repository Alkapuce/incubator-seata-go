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
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/mock"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

type mariaDBMockRows struct {
	idx  int
	data [][]interface{}
}

func (m *mariaDBMockRows) Columns() []string {
	return []string{"formatID", "gtrid_length", "bqual_length", "data"}
}

func (m *mariaDBMockRows) Close() error { return nil }

func (m *mariaDBMockRows) Next(dest []driver.Value) error {
	if m.idx == len(m.data) {
		return io.EOF
	}
	for i := 0; i < len(dest) && i < len(m.data[m.idx]); i++ {
		dest[i] = m.data[m.idx][i]
	}
	m.idx++
	return nil
}

func TestMariaDBXAResourceFactory(t *testing.T) {
	factory, ok := GetXAResourceFactory(types.DBTypeMARIADB)
	assert.True(t, ok)
	assert.IsType(t, &mariaDBXAResourceFactory{}, factory)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	resource, err := CreateXAResource(mockConn, types.DBTypeMARIADB)
	assert.NoError(t, err)
	assert.IsType(t, &MariaDBXAConn{}, resource)
	assert.IsType(t, &MariaDBXAErrorClassifier{}, CreateErrorClassifier(types.DBTypeMARIADB))
}

func TestMariaDBXAConnLifecycleSQL(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *MariaDBXAConn) error
		want string
	}{
		{
			name: "start",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.Start(ctx, "xid", TMNoFlags) },
			want: "XA START 'xid'",
		},
		{
			name: "start join",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.Start(ctx, "xid", TMJoin) },
			want: "XA START 'xid' JOIN",
		},
		{
			name: "end success",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.End(ctx, "xid", TMSuccess) },
			want: "XA END 'xid'",
		},
		{
			name: "prepare",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.XAPrepare(ctx, "xid") },
			want: "XA PREPARE 'xid'",
		},
		{
			name: "commit",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.Commit(ctx, "xid", false) },
			want: "XA COMMIT 'xid'",
		},
		{
			name: "commit one phase",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.Commit(ctx, "xid", true) },
			want: "XA COMMIT 'xid' ONE PHASE",
		},
		{
			name: "rollback",
			run:  func(ctx context.Context, conn *MariaDBXAConn) error { return conn.Rollback(ctx, "xid") },
			want: "XA ROLLBACK 'xid'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConn := mock.NewMockTestDriverConn(ctrl)
			mockConn.EXPECT().ExecContext(gomock.Any(), tt.want, gomock.Any()).Return(&driver.ResultNoRows, nil)

			conn := &MariaDBXAConn{Conn: mockConn}
			assert.NoError(t, tt.run(context.Background(), conn))
		})
	}
}

func TestMariaDBXAConnRejectsInvalidFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &MariaDBXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}
	assert.Error(t, conn.Start(context.Background(), "xid", TMSuccess))
	assert.Error(t, conn.End(context.Background(), "xid", TMJoin))
}

func TestMariaDBXAConnRecover(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockConn.EXPECT().QueryContext(gomock.Any(), "XA RECOVER", gomock.Any()).Return(&mariaDBMockRows{
		data: [][]interface{}{
			{1, 3, 0, "xid"},
			{2, 11, 0, []byte("another_xid")},
		},
	}, nil)

	conn := &MariaDBXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan|TMEndRScan)
	assert.NoError(t, err)
	assert.Equal(t, []string{"xid", "another_xid"}, got)
}

func TestMariaDBXAConnRecoverFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &MariaDBXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}

	got, err := conn.Recover(context.Background(), TMEndRScan)
	assert.NoError(t, err)
	assert.Nil(t, got)

	got, err = conn.Recover(context.Background(), TMFail)
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestMariaDBXAErrorClassifierIsAlreadyEnded(t *testing.T) {
	classifier := &MariaDBXAErrorClassifier{}

	assert.True(t, classifier.IsAlreadyEnded(&mysql.MySQLError{
		Number:  types.ErrCodeXAER_RMFAIL_IDLE,
		Message: "XAER_RMFAIL: The command cannot be executed when global transaction is in the IDLE state",
	}))
	assert.True(t, classifier.IsAlreadyEnded(&mysql.MySQLError{
		Number:  types.ErrCodeXAER_RMFAIL_IDLE,
		Message: "XAER_RMFAIL: The command cannot be executed when global transaction is in the PREPARED state",
	}))
	assert.False(t, classifier.IsAlreadyEnded(&mysql.MySQLError{
		Number:  types.ErrCodeXAER_INVAL,
		Message: "XAER_INVAL: Invalid XID",
	}))
	assert.False(t, classifier.IsAlreadyEnded(errors.New("plain error")))
}
