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

# Dameng XA Validation Checklist

This document records the validation work required before the Dameng XA resource
can move from prototype wording to supported wording. The current implementation
does not add a Dameng driver dependency. Applications must provide the selected
driver through `RegisterSeataXADriver`.

## Source Checkpoints

The current checklist is based on Dameng public documentation that should be
rechecked before a production support claim:

- [DM Go Programming Guide](https://eco.dameng.com/document/dm/zh-cn/pm/go-rogramming-guide.html)
  describes `driverName := "dm"` and the Go driver setup path.
- [DM GO interface guide](https://eco.dameng.com/document/dm/zh-cn/app-dev/go_dm.html)
  describes placing the DM GO driver under the database installation
  `drivers/go` directory before copying it into the Go source path.
- Public Git-hosted mirrors or repackaged drivers are not enough for Apache
  dependency decisions unless official ownership, versioning, license, and
  redistribution terms can be verified.
- [DM XA Programming Guide](https://eco.dameng.com/document/dm/zh-cn/pm/xa-rogramming-guide.html)
  states that `XA_COMPATIBLE_MODE=2` follows MySQL XA behavior and does not
  support the `DBMS_XA` package.

## Driver Boundary

Validate the selected driver before adding any dependency or production support
claim:

- Confirm the driver source, version, license, and redistribution terms.
- Prefer the official driver distributed with the Dameng database installation
  package unless maintainers explicitly approve another source.
- Record the driver name used with `database/sql`, the imported package path, and
  the Go version tested.
- Confirm that the driver implements the interfaces needed by this prototype:
  `driver.Conn`, `driver.ExecerContext` or `driver.ConnPrepareContext` plus
  `driver.StmtExecContext`, `driver.QueryerContext` or `driver.StmtQueryContext`,
  and `driver.ConnBeginTx`.
- Confirm whether `database/sql.Out` named output binds work for
  `DBMS_XA.XA_PREPARE`.

## Database Setup

The prototype expects an Oracle-like `DBMS_XA` API and `DBMS_XA_XID(formatid,
gtrid, bqual)` shape. Validate these database-side items on the target Dameng
deployment:

- Dameng edition and version.
- Whether the deployment enables `DBMS_XA`.
- `XA_COMPATIBLE_MODE` value and how it affects `DBMS_XA` availability.
- Required grants for the application user.
- Whether `TABLE(DBMS_XA.XA_RECOVER())` returns `formatid`, `gtrid`, and `bqual`
  in a shape that matches the prototype parser.
- Whether prepared branches remain recoverable after the application connection
  is closed.

Do not record real passwords, private hosts, wallet paths, or complete DSNs in
repository documentation.

## Lifecycle Validation

Use a disposable schema and table. At minimum, validate:

1. `XA_START` with `TMNOFLAGS`.
2. Business insert inside the XA branch.
3. `XA_END` with `TMSUCCESS`.
4. `XA_PREPARE` returning `XA_OK`.
5. `XA_RECOVER` showing the prepared branch.
6. `XA_COMMIT` making the row visible and removing the branch from recovery.
7. A separate prepared branch followed by `XA_ROLLBACK`.
8. A read-only branch where `XA_PREPARE` returns `XA_RDONLY`, if the driver and
   database support the output bind path.

## Error Semantics

Record exact errors from the selected driver and database for:

- Duplicate phase-two commit.
- Duplicate phase-two rollback.
- Branch missing during commit.
- Branch missing during rollback.
- Invalid XID format or oversized XID fields.
- Insufficient `DBMS_XA` privileges.
- Connection loss before and after prepare.

The current classifier only treats generic `XAER_NOTA` text and numeric
`DBMS_XA` return code `-4` as already-ended. Extend the classifier only after
real Dameng errors prove the additional patterns.

## Expected Support Wording

Until the checklist above is completed, use this wording:

```text
Dameng is provided as a DBMS_XA-based prototype. It has mock coverage for the
Seata Go lifecycle, readonly prepare status propagation, recovery parsing, and
phase-two cleanup, but it still requires real-driver license, compatibility
mode, recovery output, and error-code validation before production support.
```
