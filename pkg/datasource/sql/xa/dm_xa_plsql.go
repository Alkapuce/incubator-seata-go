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
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/util"
)

const dmXARecoverQuery = "SELECT x.formatid, RAWTOHEX(x.gtrid), RAWTOHEX(x.bqual) FROM TABLE(DBMS_XA.XA_RECOVER()) x"

func execDMXA(ctx context.Context, conn driver.Conn, functionName, branchXID, callArgs string, extraArgs []driver.NamedValue, allowedReturns []string) error {
	xid, err := newDMXID(branchXID)
	if err != nil {
		return err
	}

	args := []driver.NamedValue{
		{Name: "format_id", Value: int64(xid.formatID)},
		{Name: "gtrid_hex", Value: strings.ToUpper(hex.EncodeToString(xid.gtrid))},
		{Name: "bqual_hex", Value: strings.ToUpper(hex.EncodeToString(xid.bqual))},
	}
	args = append(args, extraArgs...)

	_, err = util.CtxDriverExecWithPrepareFallback(ctx, conn, buildDMXABlock(functionName, callArgs, allowedReturns), args)
	return err
}

func execDMXAWithResult(ctx context.Context, conn driver.Conn, functionName, branchXID, callArgs string, extraArgs []driver.NamedValue, allowedReturns []string) (int, error) {
	xid, err := newDMXID(branchXID)
	if err != nil {
		return 0, err
	}

	result := XAOk
	args := []driver.NamedValue{
		{Name: "format_id", Value: int64(xid.formatID)},
		{Name: "gtrid_hex", Value: strings.ToUpper(hex.EncodeToString(xid.gtrid))},
		{Name: "bqual_hex", Value: strings.ToUpper(hex.EncodeToString(xid.bqual))},
		{Name: "result", Value: gosql.Out{Dest: &result}},
	}
	args = append(args, extraArgs...)

	_, err = util.CtxDriverExecWithPrepareFallback(ctx, conn, buildDMXAResultBlock(functionName, callArgs, allowedReturns), args)
	return result, err
}

func buildDMXABlock(functionName, callArgs string, allowedReturns []string) string {
	return fmt.Sprintf(`DECLARE
  l_xid DBMS_XA_XID := DBMS_XA_XID(:format_id, HEXTORAW(:gtrid_hex), HEXTORAW(:bqual_hex));
  l_result PLS_INTEGER;
BEGIN
  l_result := DBMS_XA.%s(%s);
  IF l_result NOT IN (%s) THEN
    RAISE_APPLICATION_ERROR(-20776, 'DBMS_XA.%s failed with code ' || l_result);
  END IF;
END;`, functionName, callArgs, strings.Join(allowedReturns, ", "), functionName)
}

func buildDMXAResultBlock(functionName, callArgs string, allowedReturns []string) string {
	return fmt.Sprintf(`DECLARE
  l_xid DBMS_XA_XID := DBMS_XA_XID(:format_id, HEXTORAW(:gtrid_hex), HEXTORAW(:bqual_hex));
BEGIN
  :result := DBMS_XA.%s(%s);
  IF :result NOT IN (%s) THEN
    RAISE_APPLICATION_ERROR(-20776, 'DBMS_XA.%s failed with code ' || :result);
  END IF;
END;`, functionName, callArgs, strings.Join(allowedReturns, ", "), functionName)
}

func recoverDMXA(ctx context.Context, conn driver.Conn) ([]string, error) {
	rows, err := util.CtxDriverQueryWithPrepareFallback(ctx, conn, dmXARecoverQuery, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	xids := make([]string, 0)
	dest := make([]driver.Value, 3)
	for {
		if err = rows.Next(dest); err != nil {
			if err == io.EOF {
				return xids, nil
			}
			return nil, err
		}
		branchXID, err := dmRecoverRowBranchXID(dest)
		if err != nil {
			return nil, err
		}
		xids = append(xids, branchXID)
	}
}

func dmRecoverRowBranchXID(dest []driver.Value) (string, error) {
	if len(dest) < 3 {
		return "", fmt.Errorf("dm xa recover row has %d columns, want 3", len(dest))
	}
	formatID, err := dmIntValue(dest[0])
	if err != nil {
		return "", fmt.Errorf("parse dm xa recover format id: %w", err)
	}
	gtrid, err := dmHexValue(dest[1])
	if err != nil {
		return "", fmt.Errorf("parse dm xa recover gtrid: %w", err)
	}
	bqual, err := dmHexValue(dest[2])
	if err != nil {
		return "", fmt.Errorf("parse dm xa recover bqual: %w", err)
	}
	xid, err := newDMXIDFromParts(formatID, gtrid, bqual)
	if err != nil {
		return "", err
	}
	return xid.branchXID()
}

func dmIntValue(value driver.Value) (int, error) {
	switch v := value.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case []byte:
		return strconv.Atoi(string(v))
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("unsupported value type %T", value)
	}
}

func dmHexValue(value driver.Value) ([]byte, error) {
	var text string
	switch v := value.(type) {
	case []byte:
		text = string(v)
	case string:
		text = v
	default:
		return nil, fmt.Errorf("unsupported value type %T", value)
	}
	if text == "" {
		return nil, nil
	}
	return hex.DecodeString(text)
}
