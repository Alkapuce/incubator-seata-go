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
	"fmt"
	"strconv"
	"strings"
)

const (
	oracleXAFormatID  = 9752
	oracleXARawMaxLen = 64
)

type oracleXID struct {
	formatID int
	gtrid    []byte
	bqual    []byte
}

func newOracleXID(branchXID string) (*oracleXID, error) {
	globalXID, branchQualifier, err := splitOracleBranchXID(branchXID)
	if err != nil {
		return nil, err
	}
	return newOracleXIDFromParts(oracleXAFormatID, []byte(globalXID), []byte(branchQualifier))
}

func newOracleXIDFromParts(formatID int, gtrid, bqual []byte) (*oracleXID, error) {
	if formatID != oracleXAFormatID {
		return nil, fmt.Errorf("unsupported oracle xa format id %d", formatID)
	}
	if len(gtrid) > oracleXARawMaxLen {
		return nil, fmt.Errorf("oracle xa gtrid exceeds RAW(%d): %d", oracleXARawMaxLen, len(gtrid))
	}
	if len(bqual) > oracleXARawMaxLen {
		return nil, fmt.Errorf("oracle xa bqual exceeds RAW(%d): %d", oracleXARawMaxLen, len(bqual))
	}
	return &oracleXID{
		formatID: formatID,
		gtrid:    append([]byte(nil), gtrid...),
		bqual:    append([]byte(nil), bqual...),
	}, nil
}

func (x *oracleXID) branchXID() (string, error) {
	if x == nil {
		return "", fmt.Errorf("oracle xa xid is nil")
	}
	if x.formatID != oracleXAFormatID {
		return "", fmt.Errorf("unsupported oracle xa format id %d", x.formatID)
	}
	if len(x.gtrid) == 0 {
		return "", fmt.Errorf("oracle xa gtrid is empty")
	}
	if len(x.bqual) == 0 {
		return "", fmt.Errorf("oracle xa branch qualifier is empty")
	}
	if !strings.HasPrefix(string(x.bqual), "-") {
		return "", fmt.Errorf("oracle xa branch qualifier %q does not use seata branch id prefix", string(x.bqual))
	}
	if _, err := strconv.ParseUint(strings.TrimPrefix(string(x.bqual), "-"), 10, 64); err != nil {
		return "", fmt.Errorf("oracle xa branch qualifier %q is invalid: %w", string(x.bqual), err)
	}
	return string(x.gtrid) + string(x.bqual), nil
}

func splitOracleBranchXID(branchXID string) (string, string, error) {
	if branchXID == "" {
		return "", "", fmt.Errorf("oracle xa branch xid is empty")
	}
	idx := strings.LastIndex(branchXID, "-")
	if idx < 0 {
		return "", "", fmt.Errorf("oracle xa branch xid %q does not contain seata branch id suffix", branchXID)
	}
	if idx == 0 {
		return "", "", fmt.Errorf("oracle xa global xid is empty")
	}

	branchQualifier := branchXID[idx:]
	branchID := strings.TrimPrefix(branchQualifier, "-")
	if branchID == "" {
		return "", "", fmt.Errorf("oracle xa branch id is empty")
	}
	if _, err := strconv.ParseUint(branchID, 10, 64); err != nil {
		return "", "", fmt.Errorf("oracle xa branch id %q is invalid: %w", branchID, err)
	}
	return branchXID[:idx], branchQualifier, nil
}
