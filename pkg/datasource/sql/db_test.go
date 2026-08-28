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

package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

func TestDBResourceCheckDbVersionControlsXAConnectionHold(t *testing.T) {
	tests := []struct {
		name string
		db   *DBResource
		want bool
	}{
		{
			name: "mysql before 8.0.29 must hold",
			db:   &DBResource{dbType: types.DBTypeMySQL, dbVersion: "8.0.28"},
			want: true,
		},
		{
			name: "mysql 8.0.29 can finish phase two on another connection",
			db:   &DBResource{dbType: types.DBTypeMySQL, dbVersion: "8.0.29"},
		},
		{
			name: "postgres prepared transaction does not require held connection",
			db:   &DBResource{dbType: types.DBTypePostgreSQL, dbVersion: "PostgreSQL 15.4"},
		},
		{
			name: "mariadb must hold",
			db:   &DBResource{dbType: types.DBTypeMARIADB, dbVersion: "11.4.13-MariaDB"},
			want: true,
		},
		{
			name: "oracle dbms xa must hold until real cross-connection validation",
			db:   &DBResource{dbType: types.DBTypeOracle, dbVersion: "Oracle Database 19c"},
			want: true,
		},
		{
			name: "dm dbms xa prototype must hold until real cross-connection validation",
			db:   &DBResource{dbType: types.DBTypeDM, dbVersion: "DM8"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.db.checkDbVersion()

			assert.NoError(t, err)
			assert.Equal(t, tt.want, tt.db.IsShouldBeHeld())
			assert.Equal(t, tt.want, (&XAConn{Conn: &Conn{res: tt.db}}).ShouldBeHeld())
		})
	}
}
