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
)

func TestNewDMXID(t *testing.T) {
	xid, err := newDMXID("127.0.0.1:8091:12345-6789")

	assert.NoError(t, err)
	assert.Equal(t, dmXAFormatID, xid.formatID)
	assert.Equal(t, []byte("127.0.0.1:8091:12345"), xid.gtrid)
	assert.Equal(t, []byte("-6789"), xid.bqual)

	branchXID, err := xid.branchXID()
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8091:12345-6789", branchXID)
}

func TestNewDMXIDRejectsInvalidBranchSuffix(t *testing.T) {
	for _, input := range []string{"", "-1", "global", "global-", "global-branch"} {
		_, err := newDMXID(input)
		assert.Error(t, err)
	}
}

func TestNewDMXIDRejectsRawLengthOverflow(t *testing.T) {
	_, err := newDMXID(strings.Repeat("g", dmXARawMaxLen+1) + "-1")
	assert.Error(t, err)

	_, err = newDMXIDFromParts(dmXAFormatID, []byte("global"), []byte("-"+strings.Repeat("1", dmXARawMaxLen)))
	assert.Error(t, err)
}

func TestDMXIDBranchXIDRejectsInvalidRecoveredParts(t *testing.T) {
	tests := []struct {
		name     string
		formatID int
		gtrid    []byte
		bqual    []byte
	}{
		{name: "unsupported format id", formatID: dmXAFormatID + 1, gtrid: []byte("global"), bqual: []byte("-1")},
		{name: "empty global xid", formatID: dmXAFormatID, gtrid: nil, bqual: []byte("-1")},
		{name: "empty branch qualifier", formatID: dmXAFormatID, gtrid: []byte("global"), bqual: nil},
		{name: "bad branch qualifier prefix", formatID: dmXAFormatID, gtrid: []byte("global"), bqual: []byte("1")},
		{name: "bad branch qualifier number", formatID: dmXAFormatID, gtrid: []byte("global"), bqual: []byte("-bad")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xid := &dmXID{formatID: tt.formatID, gtrid: tt.gtrid, bqual: tt.bqual}
			_, err := xid.branchXID()
			assert.Error(t, err)
		})
	}
}
