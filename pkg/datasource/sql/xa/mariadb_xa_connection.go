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
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
	"seata.apache.org/seata-go/v2/pkg/util/log"
)

func init() {
	RegisterXAResourceFactory(types.DBTypeMARIADB, &mariaDBXAResourceFactory{})
}

type mariaDBXAResourceFactory struct{}

func (f *mariaDBXAResourceFactory) CreateXAResource(conn driver.Conn) XAResource {
	return &MariaDBXAConn{Conn: conn}
}

func (f *mariaDBXAResourceFactory) CreateErrorClassifier() XAErrorClassifier {
	return &MariaDBXAErrorClassifier{}
}

// MariaDBXAErrorClassifier classifies MariaDB XA errors reported by the
// go-sql-driver/mysql protocol driver.
type MariaDBXAErrorClassifier struct{}

func (c *MariaDBXAErrorClassifier) IsAlreadyEnded(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		if mysqlErr.Number == types.ErrCodeXAER_RMFAIL_IDLE {
			return strings.Contains(mysqlErr.Message, "IDLE state") ||
				strings.Contains(mysqlErr.Message, "PREPARED state") ||
				strings.Contains(mysqlErr.Message, "already ended")
		}
	}
	return false
}

// MariaDBXAConn implements XAResource for MariaDB using native XA SQL
// statements. MariaDB keeps MySQL-compatible XA control statements, but it has
// an independent DBType/factory so version notes, recovery behavior and error
// handling can evolve without treating MariaDB as a MySQL alias.
type MariaDBXAConn struct {
	driver.Conn
}

func (c *MariaDBXAConn) Commit(ctx context.Context, xid string, onePhase bool) error {
	log.Infof("mariadb xa branch commit, xid %s", xid)

	query := "XA COMMIT '" + xid + "'"
	if onePhase {
		query += " ONE PHASE"
	}

	conn, _ := c.Conn.(driver.ExecerContext)
	_, err := conn.ExecContext(ctx, query, nil)
	if err != nil {
		log.Errorf("mariadb xa branch commit failed, xid %s, err %v", xid, err)
	}
	return err
}

func (c *MariaDBXAConn) End(ctx context.Context, xid string, flags int) error {
	log.Infof("mariadb xa branch end, xid %s", xid)

	query := "XA END '" + xid + "'"
	switch flags {
	case TMSuccess, TMFail:
	case TMSuspend:
		query += " SUSPEND"
	default:
		return errors.New("invalid arguments")
	}

	conn, _ := c.Conn.(driver.ExecerContext)
	_, err := conn.ExecContext(ctx, query, nil)
	if err != nil {
		log.Errorf("mariadb xa branch end failed, xid %s, err %v", xid, err)
	}
	return err
}

func (c *MariaDBXAConn) Forget(ctx context.Context, xid string) error {
	return errors.New("mariadb doesn't support this")
}

func (c *MariaDBXAConn) GetTransactionTimeout() time.Duration {
	return 0
}

func (c *MariaDBXAConn) IsSameRM(ctx context.Context, xares XAResource) bool {
	return false
}

func (c *MariaDBXAConn) XAPrepare(ctx context.Context, xid string) error {
	log.Infof("mariadb xa branch prepare, xid %s", xid)

	conn, _ := c.Conn.(driver.ExecerContext)
	_, err := conn.ExecContext(ctx, "XA PREPARE '"+xid+"'", nil)
	if err != nil {
		log.Errorf("mariadb xa branch prepare failed, xid %s, err %v", xid, err)
	}
	return err
}

func (c *MariaDBXAConn) Recover(ctx context.Context, flag int) (xids []string, err error) {
	startRscan := (flag & TMStartRScan) > 0
	endRscan := (flag & TMEndRScan) > 0

	if !startRscan && !endRscan && flag != TMNoFlags {
		return nil, errors.New("invalid arguments")
	}
	if !startRscan {
		return nil, nil
	}

	conn := c.Conn.(driver.QueryerContext)
	rows, err := conn.QueryContext(ctx, "XA RECOVER", nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dest := make([]driver.Value, 4)
	for {
		if err = rows.Next(dest); err != nil {
			if err == io.EOF {
				return xids, nil
			}
			return nil, err
		}

		switch v := dest[3].(type) {
		case string:
			xids = append(xids, v)
		case []byte:
			xids = append(xids, string(v))
		default:
			return nil, errors.New("the protocol of MariaDB XA RECOVER statement is error")
		}
	}
}

func (c *MariaDBXAConn) Rollback(ctx context.Context, xid string) error {
	log.Infof("mariadb xa branch rollback, xid %s", xid)

	conn, _ := c.Conn.(driver.ExecerContext)
	_, err := conn.ExecContext(ctx, "XA ROLLBACK '"+xid+"'", nil)
	if err != nil {
		log.Errorf("mariadb xa branch rollback failed, xid %s, err %v", xid, err)
	}
	return err
}

func (c *MariaDBXAConn) SetTransactionTimeout(duration time.Duration) bool {
	return false
}

func (c *MariaDBXAConn) Start(ctx context.Context, xid string, flags int) error {
	log.Infof("mariadb xa branch start, xid %s", xid)

	query := "XA START '" + xid + "'"
	switch flags {
	case TMJoin:
		query += " JOIN"
	case TMResume:
		query += " RESUME"
	case TMNoFlags:
	default:
		return errors.New("invalid arguments")
	}

	conn, _ := c.Conn.(driver.ExecerContext)
	_, err := conn.ExecContext(ctx, query, nil)
	if err != nil {
		log.Errorf("mariadb xa branch start failed, xid %s, err %v", xid, err)
	}
	return err
}
