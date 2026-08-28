<!--
 Licensed to the Apache Software Foundation (ASF) under one or more
 contributor license agreements.  See the NOTICE file distributed with
 this work for additional information regarding copyright ownership.
 The ASF licenses this file to You under the Apache License, Version 2.0
 (the "License"); you may not use this file except in compliance with
 the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->

# XA Multi-Database Delivery Notes

This document summarizes the local delivery scope for XA multi-database support and the checks to complete before proposing Apache pull requests.

## Implemented Scope

| Area | Status |
| --- | --- |
| MariaDB | Adds `seata-xa-mariadb`, a MariaDB XA resource factory, MySQL-compatible XA lifecycle statements, recovery parsing, MariaDB-specific error classification, unit tests, an integration test, and user documentation. |
| Oracle | Adds Oracle `DBMS_XA` XID mapping, lifecycle calls, recovery parsing, prepared-statement fallback, already-ended error classification, unit tests, and setup/troubleshooting documentation. |
| Vendor adapters | Adds `RegisterSeataXADriver` and `SeataDriverDescriptor` so applications can register vendor `database/sql/driver.Driver` implementations without adding them as Seata Go dependencies. |
| Dameng prototype | Adds `types.DBTypeDM` and a `DBMS_XA`-based XA resource prototype with XID mapping, lifecycle calls, recovery parsing, error classification, unit tests, and documentation. |
| Kingbase and Oscar | Documents extension direction and open questions. Kingbase should be validated first against PostgreSQL-style prepared transactions. Oscar needs public Go driver, XA API, recovery, and error-code confirmation before code is added. |

## Verification Commands

Run the following checks before sending or updating the pull requests:

```bash
git diff --check
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType|IndexConstants' -v
go test ./pkg/datasource/sql/xa -run 'MariaDB|Oracle|DM' -v
go test ./pkg/datasource/sql/xa ./pkg/datasource/sql/types
go test ./pkg/datasource/sql/...
go test ./...
```

Run the MariaDB integration test when a MariaDB DSN is available:

```bash
SEATA_GO_TEST_MARIADB_DSN='user:password@tcp(127.0.0.1:3306)/seata_demo?parseTime=true' \
  go test -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

## Compliance Check

| Check | Current Result |
| --- | --- |
| New dependencies | No `go.mod` or `go.sum` changes are required by the current implementation. |
| Vendor drivers | Oracle and Dameng drivers are injected by applications through the vendor adapter API; they are not added as direct project dependencies. |
| License headers | New Go and Markdown files include the Apache Software Foundation license header. |
| Generated files | `dbtype_string.go` is updated together with DB type tests. |
| Secrets | Documentation examples use placeholders and must not include real DSNs, passwords, wallets, or private deployment details. |

## Suggested Pull Request Split

| PR | Scope | Notes |
| --- | --- | --- |
| PR 1 | Java/Go comparison, XA contract notes, and MariaDB resource | Keeps the first database support reviewable and includes the MariaDB integration path. |
| PR 2 | Oracle XID helper and `DBMS_XA` lifecycle | Focuses review on XID encoding, PL/SQL block construction, and lifecycle mapping. |
| PR 3 | Oracle recovery, classifier, fallback, and documentation | Groups recovery semantics, driver fallback, and user-facing Oracle setup. |
| PR 4 | Vendor XA adapter registration | Reviews public API shape for external driver injection. |
| PR 5 | Dameng prototype and domestic database extension notes | Presents Dameng as a prototype until real-driver validation is available. |

The split can be squashed differently if maintainers prefer fewer pull requests, but the review description should keep these boundaries explicit.

## Remaining Validation

| Priority | Item | Why It Matters |
| --- | --- | --- |
| P0 | Oracle real database validation | Unit tests prove the Go-side PL/SQL construction, but a real Oracle driver and database must confirm named binds, privileges, recovery rows, and error text. |
| P0 | Dameng driver and license validation | The prototype intentionally avoids a direct driver dependency until the official driver source, versioning, license, and redistribution terms are clear. |
| P1 | Dameng real database validation | `DBMS_XA` availability, `XA_COMPATIBLE_MODE`, recovery shape, and real error codes must be verified before marking Dameng production-ready. |
| P1 | MariaDB duplicate callback execution | The integration suite now includes duplicate second-phase callback coverage; run it with a real MariaDB DSN before broad support claims. |
| P2 | Kingbase prototype decision | A PostgreSQL prepared-transaction-based prototype is plausible, but it should wait for driver and compatibility confirmation. |
| P2 | Oscar follow-up | Add code only after public Go driver, XA API, recovery, and error-code evidence is available. |

## Support-Level Wording

Use precise wording in release notes and pull request descriptions:

- MariaDB: supported XA resource with unit tests, documentation, and a MariaDB integration test path.
- Oracle: `DBMS_XA` implementation with mock coverage and setup documentation; real database validation still required.
- Vendor adapter: public extension API for wrapping external drivers without adding direct dependencies.
- Dameng: prototype resource only until driver licensing, compatibility mode, recovery output, and error codes are validated on a real database.
