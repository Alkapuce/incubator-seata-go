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
)

type oracleMockRows struct {
	idx    int
	closed bool
	data   [][]interface{}
}

func (m *oracleMockRows) Columns() []string { return []string{"formatid", "gtrid", "bqual"} }

func (m *oracleMockRows) Close() error {
	m.closed = true
	return nil
}

func (m *oracleMockRows) Next(dest []driver.Value) error {
	if m.idx == len(m.data) {
		return io.EOF
	}
	for i := 0; i < len(dest) && i < len(m.data[m.idx]); i++ {
		dest[i] = m.data[m.idx][i]
	}
	m.idx++
	return nil
}

func TestOracleXAConnLifecycleExecutesDBMSXA(t *testing.T) {
	tests := []struct {
		name              string
		run               func(context.Context, *OracleXAConn) error
		wantFunction      string
		wantCall          string
		wantExtraArgName  string
		wantExtraArgValue int64
	}{
		{
			name:              "start",
			run:               func(ctx context.Context, conn *OracleXAConn) error { return conn.Start(ctx, "global-123", TMNoFlags) },
			wantFunction:      "DBMS_XA.XA_START",
			wantCall:          "DBMS_XA.XA_START(l_xid, :flag)",
			wantExtraArgName:  "flag",
			wantExtraArgValue: TMNoFlags,
		},
		{
			name:              "end success",
			run:               func(ctx context.Context, conn *OracleXAConn) error { return conn.End(ctx, "global-123", TMSuccess) },
			wantFunction:      "DBMS_XA.XA_END",
			wantCall:          "DBMS_XA.XA_END(l_xid, :flag)",
			wantExtraArgName:  "flag",
			wantExtraArgValue: TMSuccess,
		},
		{
			name:              "end fail maps to success for dbms_xa",
			run:               func(ctx context.Context, conn *OracleXAConn) error { return conn.End(ctx, "global-123", TMFail) },
			wantFunction:      "DBMS_XA.XA_END",
			wantCall:          "DBMS_XA.XA_END(l_xid, :flag)",
			wantExtraArgName:  "flag",
			wantExtraArgValue: TMSuccess,
		},
		{
			name:         "prepare",
			run:          func(ctx context.Context, conn *OracleXAConn) error { return conn.XAPrepare(ctx, "global-123") },
			wantFunction: "DBMS_XA.XA_PREPARE",
			wantCall:     "DBMS_XA.XA_PREPARE(l_xid)",
		},
		{
			name:         "commit",
			run:          func(ctx context.Context, conn *OracleXAConn) error { return conn.Commit(ctx, "global-123", false) },
			wantFunction: "DBMS_XA.XA_COMMIT",
			wantCall:     "DBMS_XA.XA_COMMIT(l_xid, FALSE)",
		},
		{
			name:         "commit one phase",
			run:          func(ctx context.Context, conn *OracleXAConn) error { return conn.Commit(ctx, "global-123", true) },
			wantFunction: "DBMS_XA.XA_COMMIT",
			wantCall:     "DBMS_XA.XA_COMMIT(l_xid, TRUE)",
		},
		{
			name:         "rollback",
			run:          func(ctx context.Context, conn *OracleXAConn) error { return conn.Rollback(ctx, "global-123") },
			wantFunction: "DBMS_XA.XA_ROLLBACK",
			wantCall:     "DBMS_XA.XA_ROLLBACK(l_xid)",
		},
		{
			name:         "forget",
			run:          func(ctx context.Context, conn *OracleXAConn) error { return conn.Forget(ctx, "global-123") },
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
					assert.Equal(t, int64(oracleXAFormatID), argByName(t, args, "format_id").Value)
					assert.Equal(t, "676C6F62616C", argByName(t, args, "gtrid_hex").Value)
					assert.Equal(t, "2D313233", argByName(t, args, "bqual_hex").Value)
					if tt.wantExtraArgName != "" {
						assert.Equal(t, tt.wantExtraArgValue, argByName(t, args, tt.wantExtraArgName).Value)
					}
					return &driver.ResultNoRows, nil
				})

			conn := &OracleXAConn{Conn: mockConn}
			assert.NoError(t, tt.run(context.Background(), conn))
		})
	}
}

func TestOracleXAPrepareAcceptsReadonly(t *testing.T) {
	block := buildOracleXABlock("XA_PREPARE", "l_xid", []string{"DBMS_XA.XA_OK", "DBMS_XA.XA_RDONLY"})
	statusBlock := buildOracleXAResultBlock("XA_PREPARE", "l_xid", []string{"DBMS_XA.XA_OK", "DBMS_XA.XA_RDONLY"})

	assert.Contains(t, block, "IF l_result NOT IN (DBMS_XA.XA_OK, DBMS_XA.XA_RDONLY) THEN")
	assert.Contains(t, statusBlock, ":result := DBMS_XA.XA_PREPARE(l_xid)")
	assert.Contains(t, statusBlock, "IF :result NOT IN (DBMS_XA.XA_OK, DBMS_XA.XA_RDONLY) THEN")
}

func TestOracleXAPrepareStatusReturnsReadonly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockConn.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
			assert.Contains(t, query, ":result := DBMS_XA.XA_PREPARE(l_xid)")
			out, ok := argByName(t, args, "result").Value.(gosql.Out)
			if assert.True(t, ok) {
				result, ok := out.Dest.(*int)
				if assert.True(t, ok) {
					*result = XAReadOnly
				}
			}
			return &driver.ResultNoRows, nil
		})

	conn := &OracleXAConn{Conn: mockConn}
	status, err := conn.XAPrepareStatus(context.Background(), "global-123")
	assert.NoError(t, err)
	assert.Equal(t, XAReadOnly, status)
}

func TestOracleXAExecFallsBackToPrepare(t *testing.T) {
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
			assert.Equal(t, int64(oracleXAFormatID), argByName(t, args, "format_id").Value)
			assert.Equal(t, "676C6F62616C", argByName(t, args, "gtrid_hex").Value)
			assert.Equal(t, "2D313233", argByName(t, args, "bqual_hex").Value)
			assert.IsType(t, gosql.Out{}, argByName(t, args, "result").Value)
			return &driver.ResultNoRows, nil
		})
	mockStmt.EXPECT().Close().Return(nil)

	conn := &OracleXAConn{Conn: mockConn}
	assert.NoError(t, conn.XAPrepare(context.Background(), "global-123"))
}

func TestOracleXAConnRejectsInvalidFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &OracleXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}
	assert.Error(t, conn.Start(context.Background(), "global-123", TMSuccess))
	assert.Error(t, conn.End(context.Background(), "global-123", TMJoin))
}

func TestOracleXAConnRecover(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	rows := &oracleMockRows{
		data: [][]interface{}{
			{int64(oracleXAFormatID), "676C6F62616C", "2D313233"},
			{[]byte("9752"), []byte("616E6F74686572"), []byte("2D343536")},
		},
	}
	mockConn.EXPECT().QueryContext(gomock.Any(), oracleXARecoverQuery, gomock.Any()).Return(rows, nil)

	conn := &OracleXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan|TMEndRScan)
	assert.NoError(t, err)
	assert.Equal(t, []string{"global-123", "another-456"}, got)
	assert.True(t, rows.closed)
}

func TestOracleXARecoverFallsBackToPrepare(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockStmt := mock.NewMockTestDriverStmt(ctrl)
	mockRows := &oracleMockRows{
		data: [][]interface{}{{int64(oracleXAFormatID), "676C6F62616C", "2D313233"}},
	}
	mockConn.EXPECT().QueryContext(gomock.Any(), oracleXARecoverQuery, gomock.Any()).Return(nil, driver.ErrSkip)
	mockConn.EXPECT().PrepareContext(gomock.Any(), oracleXARecoverQuery).Return(mockStmt, nil)
	mockStmt.EXPECT().QueryContext(gomock.Any(), gomock.Any()).Return(mockRows, nil)
	mockStmt.EXPECT().Close().Return(nil)

	conn := &OracleXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan)
	assert.NoError(t, err)
	assert.Equal(t, []string{"global-123"}, got)
	assert.True(t, mockRows.closed)
}

func TestOracleXAConnRecoverFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &OracleXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}

	got, err := conn.Recover(context.Background(), TMEndRScan)
	assert.NoError(t, err)
	assert.Nil(t, got)

	got, err = conn.Recover(context.Background(), TMFail)
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestOracleXAConnRecoverRejectsInvalidProtocol(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mock.NewMockTestDriverConn(ctrl)
	mockConn.EXPECT().QueryContext(gomock.Any(), oracleXARecoverQuery, gomock.Any()).Return(&oracleMockRows{
		data: [][]interface{}{{int64(oracleXAFormatID), "676C6F62616C", "BAD"}},
	}, nil)

	conn := &OracleXAConn{Conn: mockConn}
	got, err := conn.Recover(context.Background(), TMStartRScan)
	assert.Nil(t, got)
	assert.Error(t, err)
}

func TestOracleXAErrorClassifierIsAlreadyEnded(t *testing.T) {
	classifier := &OracleXAErrorClassifier{}

	assert.True(t, classifier.IsAlreadyEnded(errors.New("ORA-24756: transaction does not exist")))
	assert.True(t, classifier.IsAlreadyEnded(errors.New("ORA-24761: transaction rolled back")))
	assert.True(t, classifier.IsAlreadyEnded(errors.New("DBMS_XA.XA_COMMIT failed with code XAER_NOTA")))
	assert.True(t, classifier.IsAlreadyEnded(errors.New("ORA-20777: DBMS_XA.XA_COMMIT failed with code -4, oracle error 0")))
	assert.True(t, classifier.IsAlreadyEnded(errors.New("ORA-20777: DBMS_XA.XA_ROLLBACK failed with code -3, oracle error 24756")))
	assert.False(t, classifier.IsAlreadyEnded(errors.New("ORA-01031: insufficient privileges")))
	assert.False(t, classifier.IsAlreadyEnded(errors.New("ORA-20777: DBMS_XA.XA_COMMIT failed with code -5, oracle error 0")))
	assert.False(t, classifier.IsAlreadyEnded(nil))
}

func argByName(t *testing.T, args []driver.NamedValue, name string) driver.NamedValue {
	t.Helper()

	for _, arg := range args {
		if arg.Name == name {
			return arg
		}
	}
	require.Failf(t, "missing named arg", "arg %s not found in %v", name, args)
	return driver.NamedValue{}
}
