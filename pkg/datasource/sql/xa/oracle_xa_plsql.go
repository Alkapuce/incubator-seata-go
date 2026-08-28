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
	"encoding/hex"
	"fmt"
	"strings"
)

const oracleXATransLoose = 0x00010000

func execOracleXA(ctx context.Context, conn driver.Conn, functionName, branchXID, callArgs string, extraArgs []driver.NamedValue, allowedReturns []string) error {
	execer, ok := conn.(driver.ExecerContext)
	if !ok {
		return fmt.Errorf("oracle xa requires driver.ExecerContext, got %T", conn)
	}

	xid, err := newOracleXID(branchXID)
	if err != nil {
		return err
	}

	args := []driver.NamedValue{
		{Name: "format_id", Value: int64(xid.formatID)},
		{Name: "gtrid_hex", Value: strings.ToUpper(hex.EncodeToString(xid.gtrid))},
		{Name: "bqual_hex", Value: strings.ToUpper(hex.EncodeToString(xid.bqual))},
	}
	args = append(args, extraArgs...)

	_, err = execer.ExecContext(ctx, buildOracleXABlock(functionName, callArgs, allowedReturns), args)
	return err
}

func buildOracleXABlock(functionName, callArgs string, allowedReturns []string) string {
	return fmt.Sprintf(`DECLARE
  l_xid DBMS_XA_XID := DBMS_XA_XID(:format_id, HEXTORAW(:gtrid_hex), HEXTORAW(:bqual_hex));
  l_result PLS_INTEGER;
BEGIN
  l_result := DBMS_XA.%s(%s);
  IF l_result NOT IN (%s) THEN
    RAISE_APPLICATION_ERROR(-20777, 'DBMS_XA.%s failed with code ' || l_result || ', oracle error ' || DBMS_XA.XA_GETLASTOER());
  END IF;
END;`, functionName, callArgs, strings.Join(allowedReturns, ", "), functionName)
}
