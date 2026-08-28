//go:build integration
// +build integration

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
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

func TestMariaDBXAConnIntegrationCommitRollbackRecover(t *testing.T) {
	dsn := os.Getenv("SEATA_GO_TEST_MARIADB_DSN")
	if dsn == "" {
		t.Skip("SEATA_GO_TEST_MARIADB_DSN is not set")
	}

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx))

	tableName := "seata_go_mariadb_xa_it"
	_, err = db.ExecContext(ctx, "DROP TABLE IF EXISTS "+tableName)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "CREATE TABLE "+tableName+" (id INT PRIMARY KEY, value VARCHAR(64)) ENGINE=InnoDB")
	require.NoError(t, err)
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS "+tableName)

	require.NoError(t, withMariaDBXAResource(ctx, db, func(resource XAResource, execer driver.ExecerContext) error {
		commitXID := uniqueMariaDBXID(t, "commit")
		if err := resource.Start(ctx, commitXID, TMNoFlags); err != nil {
			return err
		}
		if _, err := execer.ExecContext(ctx, "INSERT INTO "+tableName+" (id, value) VALUES (1, 'commit')", nil); err != nil {
			return err
		}
		if err := resource.End(ctx, commitXID, TMSuccess); err != nil {
			return err
		}
		if err := resource.XAPrepare(ctx, commitXID); err != nil {
			return err
		}
		prepared, err := resource.Recover(ctx, TMStartRScan|TMEndRScan)
		if err != nil {
			return err
		}
		if !containsXID(prepared, commitXID) {
			return fmt.Errorf("prepared xid %s not found in XA RECOVER result %v", commitXID, prepared)
		}
		return resource.Commit(ctx, commitXID, false)
	}))

	require.Equal(t, 1, countRowsByID(t, db, tableName, 1))

	require.NoError(t, withMariaDBXAResource(ctx, db, func(resource XAResource, execer driver.ExecerContext) error {
		rollbackXID := uniqueMariaDBXID(t, "rollback")
		if err := resource.Start(ctx, rollbackXID, TMNoFlags); err != nil {
			return err
		}
		if _, err := execer.ExecContext(ctx, "INSERT INTO "+tableName+" (id, value) VALUES (2, 'rollback')", nil); err != nil {
			return err
		}
		if err := resource.End(ctx, rollbackXID, TMSuccess); err != nil {
			return err
		}
		if err := resource.XAPrepare(ctx, rollbackXID); err != nil {
			return err
		}
		prepared, err := resource.Recover(ctx, TMStartRScan|TMEndRScan)
		if err != nil {
			return err
		}
		if !containsXID(prepared, rollbackXID) {
			return fmt.Errorf("prepared xid %s not found in XA RECOVER result %v", rollbackXID, prepared)
		}
		return resource.Rollback(ctx, rollbackXID)
	}))

	require.Equal(t, 0, countRowsByID(t, db, tableName, 2))
}

func withMariaDBXAResource(ctx context.Context, db *sql.DB, f func(XAResource, driver.ExecerContext) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		conn, ok := driverConn.(driver.Conn)
		if !ok {
			return fmt.Errorf("driver connection does not implement driver.Conn: %T", driverConn)
		}
		execer, ok := driverConn.(driver.ExecerContext)
		if !ok {
			return fmt.Errorf("driver connection does not implement driver.ExecerContext: %T", driverConn)
		}
		resource, err := CreateXAResource(conn, types.DBTypeMARIADB)
		if err != nil {
			return err
		}
		return f(resource, execer)
	})
}

func uniqueMariaDBXID(t *testing.T, label string) string {
	t.Helper()

	var suffix [4]byte
	_, err := rand.Read(suffix[:])
	require.NoError(t, err)
	return fmt.Sprintf("seata-go-mariadb-%s-%d-%x", label, time.Now().UnixNano(), suffix)
}

func containsXID(xids []string, xid string) bool {
	for _, candidate := range xids {
		if candidate == xid {
			return true
		}
	}
	return false
}

func countRowsByID(t *testing.T, db *sql.DB, tableName string, id int) int {
	t.Helper()

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM "+tableName+" WHERE id = ?", id).Scan(&count)
	require.NoError(t, err)
	return count
}
