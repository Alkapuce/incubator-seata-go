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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOracleXID(t *testing.T) {
	xid, err := newOracleXID("127.0.0.1:8091:12345-6789")
	require.NoError(t, err)

	assert.Equal(t, oracleXAFormatID, xid.formatID)
	assert.Equal(t, []byte("127.0.0.1:8091:12345"), xid.gtrid)
	assert.Equal(t, []byte("-6789"), xid.bqual)

	got, err := xid.branchXID()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8091:12345-6789", got)
}

func TestNewOracleXIDRejectsInvalidBranchSuffix(t *testing.T) {
	tests := []string{
		"",
		"-1",
		"global",
		"global-",
		"global-branch",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := newOracleXID(input)
			assert.Error(t, err)
		})
	}
}

func TestNewOracleXIDRejectsRawLengthOverflow(t *testing.T) {
	_, err := newOracleXID(strings.Repeat("g", oracleXARawMaxLen+1) + "-1")
	assert.ErrorContains(t, err, "gtrid exceeds")

	_, err = newOracleXIDFromParts(oracleXAFormatID, []byte("global"), []byte("-"+strings.Repeat("1", oracleXARawMaxLen)))
	assert.ErrorContains(t, err, "bqual exceeds")
}

func TestOracleXIDBranchXIDRejectsInvalidRecoveredParts(t *testing.T) {
	tests := []struct {
		name string
		xid  *oracleXID
	}{
		{
			name: "unsupported format id",
			xid:  &oracleXID{formatID: oracleXAFormatID + 1, gtrid: []byte("global"), bqual: []byte("-1")},
		},
		{
			name: "empty global xid",
			xid:  &oracleXID{formatID: oracleXAFormatID, bqual: []byte("-1")},
		},
		{
			name: "empty branch qualifier",
			xid:  &oracleXID{formatID: oracleXAFormatID, gtrid: []byte("global")},
		},
		{
			name: "bad branch qualifier prefix",
			xid:  &oracleXID{formatID: oracleXAFormatID, gtrid: []byte("global"), bqual: []byte("1")},
		},
		{
			name: "bad branch qualifier number",
			xid:  &oracleXID{formatID: oracleXAFormatID, gtrid: []byte("global"), bqual: []byte("-bad")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.xid.branchXID()
			assert.Empty(t, got)
			assert.Error(t, err)
		})
	}
}
