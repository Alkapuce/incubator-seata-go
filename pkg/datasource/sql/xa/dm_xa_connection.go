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
	"strings"
	"time"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
	"seata.apache.org/seata-go/v2/pkg/util/log"
)

func init() {
	RegisterXAResourceFactory(types.DBTypeDM, &dmXAResourceFactory{})
}

type dmXAResourceFactory struct{}

func (f *dmXAResourceFactory) CreateXAResource(conn driver.Conn) XAResource {
	return &DMXAConn{Conn: conn}
}

func (f *dmXAResourceFactory) CreateErrorClassifier() XAErrorClassifier {
	return &DMXAErrorClassifier{}
}

// DMXAErrorClassifier classifies Dameng DBMS_XA errors reported by the driver.
type DMXAErrorClassifier struct{}

func (c *DMXAErrorClassifier) IsAlreadyEnded(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "XAER_NOTA") ||
		strings.Contains(msg, "FAILED WITH CODE -4")
}

// DMXAConn implements a Dameng XAResource prototype using DBMS_XA PL/SQL calls.
//
// The implementation intentionally has no direct Dameng driver dependency.
// Applications register their selected driver with RegisterSeataXADriver and
// DBTypeDM, while the resource handles the DBMS_XA lifecycle contract.
type DMXAConn struct {
	driver.Conn
}

func (c *DMXAConn) Start(ctx context.Context, xid string, flags int) error {
	log.Infof("dm xa branch start, xid %s", xid)
	flags, err := dmXAStartFlag(flags)
	if err != nil {
		return err
	}
	return execDMXA(ctx, c.Conn, "XA_START", xid, "l_xid, :flag", []driver.NamedValue{
		{Name: "flag", Value: int64(flags)},
	}, []string{"DBMS_XA.XA_OK"})
}

func (c *DMXAConn) End(ctx context.Context, xid string, flags int) error {
	log.Infof("dm xa branch end, xid %s", xid)
	flags, err := dmXAEndFlag(flags)
	if err != nil {
		return err
	}
	return execDMXA(ctx, c.Conn, "XA_END", xid, "l_xid, :flag", []driver.NamedValue{
		{Name: "flag", Value: int64(flags)},
	}, []string{"DBMS_XA.XA_OK"})
}

func (c *DMXAConn) XAPrepare(ctx context.Context, xid string) error {
	_, err := c.XAPrepareStatus(ctx, xid)
	return err
}

func (c *DMXAConn) XAPrepareStatus(ctx context.Context, xid string) (int, error) {
	log.Infof("dm xa branch prepare, xid %s", xid)
	return execDMXAWithResult(ctx, c.Conn, "XA_PREPARE", xid, "l_xid", nil, []string{
		"DBMS_XA.XA_OK",
		"DBMS_XA.XA_RDONLY",
	})
}

func (c *DMXAConn) Commit(ctx context.Context, xid string, onePhase bool) error {
	log.Infof("dm xa branch commit, xid %s, onePhase %v", xid, onePhase)
	onePhaseArg := "FALSE"
	if onePhase {
		onePhaseArg = "TRUE"
	}
	return execDMXA(ctx, c.Conn, "XA_COMMIT", xid, "l_xid, "+onePhaseArg, nil, []string{"DBMS_XA.XA_OK"})
}

func (c *DMXAConn) Rollback(ctx context.Context, xid string) error {
	log.Infof("dm xa branch rollback, xid %s", xid)
	return execDMXA(ctx, c.Conn, "XA_ROLLBACK", xid, "l_xid", nil, []string{"DBMS_XA.XA_OK"})
}

func (c *DMXAConn) Recover(ctx context.Context, flag int) ([]string, error) {
	startRscan := (flag & TMStartRScan) > 0
	endRscan := (flag & TMEndRScan) > 0

	if !startRscan && !endRscan && flag != TMNoFlags {
		return nil, errors.New("invalid arguments")
	}
	if !startRscan {
		return nil, nil
	}
	return recoverDMXA(ctx, c.Conn)
}

func (c *DMXAConn) Forget(ctx context.Context, xid string) error {
	return execDMXA(ctx, c.Conn, "XA_FORGET", xid, "l_xid", nil, []string{"DBMS_XA.XA_OK"})
}

func (c *DMXAConn) GetTransactionTimeout() time.Duration { return 0 }

func (c *DMXAConn) IsSameRM(ctx context.Context, resource XAResource) bool { return false }

func (c *DMXAConn) SetTransactionTimeout(duration time.Duration) bool { return false }

func dmXAStartFlag(flags int) (int, error) {
	switch flags {
	case TMNoFlags, TMJoin, TMResume:
		return flags, nil
	default:
		return 0, errors.New("invalid arguments")
	}
}

func dmXAEndFlag(flags int) (int, error) {
	switch flags {
	case TMSuccess, TMSuspend:
		return flags, nil
	case TMFail:
		return TMSuccess, nil
	default:
		return 0, errors.New("invalid arguments")
	}
}
