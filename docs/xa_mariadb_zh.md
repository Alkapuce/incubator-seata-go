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

# MariaDB XA 使用指南

Seata Go 提供独立的 MariaDB XA driver 名称：

```go
db, err := sql.Open(
	"seata-xa-mariadb",
	"root:password@tcp(127.0.0.1:3306)/seata_demo?charset=utf8mb4&parseTime=True&multiStatements=true",
)
```

MariaDB 支持复用已有 MySQL wire protocol driver 依赖，并使用 MariaDB 与 MySQL 兼容的 XA 控制语句：

- `XA START`
- `XA END`
- `XA PREPARE`
- `XA COMMIT`
- `XA ROLLBACK`
- `XA RECOVER`

## 使用要求

- 使用支持 XA transaction 的 MariaDB 版本。
- 业务表使用 InnoDB 等事务型存储引擎。
- 应用用户需要具备业务 SQL 和 XA 控制语句所需权限。
- 如果应用会通过同一连接路径执行多语句，DSN 中保留 `multiStatements=true`。

## 已知行为

MariaDB 拥有独立的 `DBTypeMARIADB`、resource factory 和 error classifier。即使第一版实现使用与
MySQL 兼容的 XA SQL，也不把 MariaDB 当成 MySQL alias，这样后续可以独立维护 MariaDB 版本差异、
recover 行为和错误处理。

MariaDB XA 连接会保留到二阶段完成。这与 Seata Java 的 MariaDB 行为一致，也与 Go 侧现有
`DBResource.checkDbVersion()` 策略一致。

## 本地集成测试

MariaDB 集成测试默认不会运行。设置 `SEATA_GO_TEST_MARIADB_DSN` 后，用 `integration` build tag 执行：

```bash
SEATA_GO_TEST_MARIADB_DSN='root:password@tcp(127.0.0.1:3306)/seata_go_test?multiStatements=true&parseTime=true' \
  go test -count=1 -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

这些集成测试会验证 prepare 后提交分支、prepare 后回滚分支、`XA RECOVER` 结果、prepared branch 清理，以及重复二阶段回调行为。

## 故障排查

| 现象 | 检查项 |
| --- | --- |
| `no XA resource factory registered for db type` | 确认应用已导入并初始化 Seata Go SQL 支持，并使用 `seata-xa-mariadb`。 |
| XA resource 方法返回 `invalid arguments` | 检查调用方传入的 XA flag。`Start` 接受 `TMNoFlags`、`TMJoin`、`TMResume`；`End` 接受 `TMSuccess`、`TMFail`、`TMSuspend`。 |
| 故障后仍有 prepared branch | 在 MariaDB 执行 `XA RECOVER`，确认全局事务决议后用 `XA COMMIT '<xid>'` 或 `XA ROLLBACK '<xid>'` 释放分支。 |
| XA 语句在非事务表上失败 | 使用 InnoDB 等事务型存储引擎。 |
