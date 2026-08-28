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
	gosql "database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/mock"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

type dmMockRows struct {
	idx    int
	closed bool
	data   [][]interface{}
}

func (m *dmMockRows) Columns() []string { return []string{"formatid", "gtrid", "bqual"} }

func (m *dmMockRows) Close() error {
	m.closed = true
	return nil
}

func (m *dmMockRows) Next(dest []driver.Value) error {
	if m.idx == len(m.data) {
		return io.EOF
	}
	for i := 0; i < len(dest) && i < len(m.data[m.idx]); i++ {
		dest[i] = m.data[m.idx][i]
	}
	m.idx++
	return nil
}

func TestDMXAResourceFactory(t *testing.T) {
	factory, ok := GetXAResourceFactory(types.DBTypeDM)
	assert.True(t, ok)
	assert.IsType(t, &dmXAResourceFactory{}, factory)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	resource, err := CreateXAResource(mockConn, types.DBTypeDM)
	assert.NoError(t, err)
	assert.IsType(t, &DMXAConn{}, resource)
	assert.IsType(t, &DMXAErrorClassifier{}, CreateErrorClassifier(types.DBTypeDM))
}

func TestDMXAConnLifecycleExecutesDBMSXA(t *testing.T) {
	tests := []struct {
		name              string
		run               func(context.Context, *DMXAConn) error
		wantFunction      string
		wantCall          string
		wantExtraArgName  string
		wantExtraArgValue int64
	}{
		{
			name:              "start",
			run:               func(ctx context.Context, conn *DMXAConn) error { return conn.Start(ctx, "global-123", TMNoFlags) },
			wantFunction:      "DBMS_XA.XA_START",
			wantCall:          "DBMS_XA.XA_START(l_xid, :flag)",
			wantExtraArgName:  "flag",
			wantExtraArgValue: TMNoFlags,
		},
		{
			name:              "end success",
			run:               func(ctx context.Context, conn *DMXAConn) error { return conn.End(ctx, "global-123", TMSuccess) },
			wantFunction:      "DBMS_XA.XA_END",
			wantCall:          "DBMS_XA.XA_END(l_xid, :flag)",
			wantExtraArgName:  "flag",
			wantExtraArgValue: TMSuccess,
		},
		{
			name:              "end fail maps to success for dbms_xa",
			run:               func(ctx context.Context, conn *DMXAConn) error { return conn.End(ctx, "global-123", TMFail) },
			wantFunction:      "DBMS_XA.XA_END",
			wantCall:          "DBMS_XA.XA_END(l_xid, :flag)",
			wantExtraArgName:  "flag",
			wantExtraArgValue: TMSuccess,
		},
		{
			name:         "prepare",
			run:          func(ctx context.Context, conn *DMXAConn) error { return conn.XAPrepare(ctx, "global-123") },
			wantFunction: "DBMS_XA.XA_PREPARE",
			wantCall:     "DBMS_XA.XA_PREPARE(l_xid)",
		},
		{
			name:         "commit",
			run:          func(ctx context.Context, conn *DMXAConn) error { return conn.Commit(ctx, "global-123", false) },
			wantFunction: "DBMS_XA.XA_COMMIT",
			wantCall:     "DBMS_XA.XA_COMMIT(l_xid, FALSE)",
		},
		{
			name:         "rollback",
			run:          func(ctx context.Context, conn *DMXAConn) error { return conn.Rollback(ctx, "global-123") },
			wantFunction: "DBMS_XA.XA_ROLLBACK",
			wantCall:     "DBMS_XA.XA_ROLLBACK(l_xid)",
		},
		{
			name:         "forget",
			run:          func(ctx context.Context, conn *DMXAConn) error { return conn.Forget(ctx, "global-123") },
			wantFunction: "DBMS_XA.XA_FORGET",
			wantCall:     "DBMS_XA.XA_FORGET(l_xid)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConn := mock.NewMockTestDriverConn(ctrl)
			mockConn.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
					assert.Contains(t, query, tt.wantFunction)
					assert.Contains(t, query, tt.wantCall)
					assert.Contains(t, query, "DBMS_XA_XID(:format_id, HEXTORAW(:gtrid_hex), HEXTORAW(:bqual_hex))")
					assert.Equal(t, int64(dmXAFormatID), dmArgByName(t, args, "format_id").Value)
					assert.Equal(t, "676C6F62616C", dmArgByName(t, args, "gtrid_hex").Value)
					assert.Equal(t, "2D313233", dmArgByName(t, args, "bqual_hex").Value)
					if tt.wantExtraArgName != "" {
						assert.Equal(t, tt.wantExtraArgValue, dmArgByName(t, args, tt.wantExtraArgName).Value)
					}
					return &driver.ResultNoRows, nil
				})

			conn := &DMXAConn{Conn: mockConn}
			assert.NoError(t, tt.run(context.Background(), conn))
		})
	}
}

func TestDMXAPrepareAcceptsReadonly(t *testing.T) {
	block := buildDMXABlock("XA_PREPARE", "l_xid", []string{"DBMS_XA.XA_OK", "DBMS_XA.XA_RDONLY"})
	statusBlock := buildDMXAResultBlock("XA_PREPARE", "l_xid", []string{"DBMS_XA.XA_OK", "DBMS_XA.XA_RDONLY"})

	assert.Contains(t, block, "IF l_result NOT IN (DBMS_XA.XA_OK, DBMS_XA.XA_RDONLY) THEN")
	assert.Contains(t, statusBlock, ":result := DBMS_XA.XA_PREPARE(l_xid)")
	assert.Contains(t, statusBlock, "IF :result NOT IN (DBMS_XA.XA_OK, DBMS_XA.XA_RDONLY) THEN")
}

func TestDMXAPrepareStatusReturnsReadonly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockConn.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
			assert.Contains(t, query, ":result := DBMS_XA.XA_PREPARE(l_xid)")
			out, ok := dmArgByName(t, args, "result").Value.(gosql.Out)
			if assert.True(t, ok) {
				result, ok := out.Dest.(*int)
				if assert.True(t, ok) {
					*result = XAReadOnly
				}
			}
			return &driver.ResultNoRows, nil
		})

	conn := &DMXAConn{Conn: mockConn}
	status, err := conn.XAPrepareStatus(context.Background(), "global-123")
	assert.NoError(t, err)
	assert.Equal(t, XAReadOnly, status)
}

func TestDMXAExecFallsBackToPrepare(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockStmt := mock.NewMockTestDriverStmt(ctrl)
	mockConn.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, driver.ErrSkip)
	mockConn.EXPECT().PrepareContext(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, query string) (driver.Stmt, error) {
			assert.Contains(t, query, "DBMS_XA.XA_PREPARE(l_xid)")
			return mockStmt, nil
		})
	mockStmt.EXPECT().ExecContext(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
			assert.Equal(t, int64(dmXAFormatID), dmArgByName(t, args, "format_id").Value)
			assert.Equal(t, "676C6F62616C", dmArgByName(t, args, "gtrid_hex").Value)
			assert.Equal(t, "2D313233", dmArgByName(t, args, "bqual_hex").Value)
			assert.IsType(t, gosql.Out{}, dmArgByName(t, args, "result").Value)
			return &driver.ResultNoRows, nil
		})
	mockStmt.EXPECT().Close().Return(nil)

	conn := &DMXAConn{Conn: mockConn}
	assert.NoError(t, conn.XAPrepare(context.Background(), "global-123"))
}

func TestDMXAConnRejectsInvalidFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &DMXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}
	assert.Error(t, conn.Start(context.Background(), "global-123", TMSuccess))
	assert.Error(t, conn.End(context.Background(), "global-123", TMJoin))
}

func TestDMXAConnRecover(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	rows := &dmMockRows{
		data: [][]interface{}{
			{int64(dmXAFormatID), "676C6F62616C", "2D313233"},
			{[]byte("9752"), []byte("616E6F74686572"), []byte("2D343536")},
		},
	}
	mockConn.EXPECT().QueryContext(gomock.Any(), dmXARecoverQuery, gomock.Any()).Return(rows, nil)

	conn := &DMXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan|TMEndRScan)
	assert.NoError(t, err)
	assert.Equal(t, []string{"global-123", "another-456"}, got)
	assert.True(t, rows.closed)
}

func TestDMXARecoverFallsBackToPrepare(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockStmt := mock.NewMockTestDriverStmt(ctrl)
	mockRows := &dmMockRows{
		data: [][]interface{}{{int64(dmXAFormatID), "676C6F62616C", "2D313233"}},
	}
	mockConn.EXPECT().QueryContext(gomock.Any(), dmXARecoverQuery, gomock.Any()).Return(nil, driver.ErrSkip)
	mockConn.EXPECT().PrepareContext(gomock.Any(), dmXARecoverQuery).Return(mockStmt, nil)
	mockStmt.EXPECT().QueryContext(gomock.Any(), gomock.Any()).Return(mockRows, nil)
	mockStmt.EXPECT().Close().Return(nil)

	conn := &DMXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan)
	assert.NoError(t, err)
	assert.Equal(t, []string{"global-123"}, got)
	assert.True(t, mockRows.closed)
}

func TestDMXAConnRecoverFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &DMXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}

	got, err := conn.Recover(context.Background(), TMEndRScan)
	assert.NoError(t, err)
	assert.Nil(t, got)

	got, err = conn.Recover(context.Background(), TMFail)
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestDMXAConnRecoverRejectsInvalidProtocol(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockConn.EXPECT().QueryContext(gomock.Any(), dmXARecoverQuery, gomock.Any()).Return(&dmMockRows{
		data: [][]interface{}{{int64(dmXAFormatID), "676C6F62616C", "BAD"}},
	}, nil)

	conn := &DMXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan)
	assert.Nil(t, got)
	assert.Error(t, err)
}

func TestDMXAErrorClassifierIsAlreadyEnded(t *testing.T) {
	classifier := &DMXAErrorClassifier{}

	assert.True(t, classifier.IsAlreadyEnded(errors.New("DBMS_XA.XA_COMMIT failed with code XAER_NOTA")))
	assert.True(t, classifier.IsAlreadyEnded(errors.New("DM DBMS_XA.XA_COMMIT failed with code -4")))
	assert.False(t, classifier.IsAlreadyEnded(errors.New("DM DBMS_XA.XA_COMMIT failed with code -5")))
	assert.False(t, classifier.IsAlreadyEnded(nil))
}

func dmArgByName(t *testing.T, args []driver.NamedValue, name string) driver.NamedValue {
	t.Helper()

	for _, arg := range args {
		if arg.Name == name {
			return arg
		}
	}
	require.Failf(t, "missing named arg", "arg %s not found in %v", name, args)
	return driver.NamedValue{}
}
