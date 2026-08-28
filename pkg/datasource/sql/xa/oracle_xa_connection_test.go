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
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/mock"
)

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
			wantExtraArgValue: oracleXATransLoose,
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

	assert.Contains(t, block, "IF l_result NOT IN (DBMS_XA.XA_OK, DBMS_XA.XA_RDONLY) THEN")
}

func TestOracleXAConnRejectsInvalidFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := &OracleXAConn{Conn: mock.NewMockTestDriverConn(ctrl)}
	assert.Error(t, conn.Start(context.Background(), "global-123", TMSuccess))
	assert.Error(t, conn.End(context.Background(), "global-123", TMJoin))
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
