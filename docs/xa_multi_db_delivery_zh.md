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

# XA 多数据库交付说明

本文汇总 XA 多数据库支持的本地交付范围，以及提交 Apache PR 前需要完成的检查。

提交 PR 前，使用 [`xa_multi_db_pr_readiness_zh.md`](./xa_multi_db_pr_readiness_zh.md)
逐项核对原始要求、实现证据、验证命令和剩余边界。

## 已实现范围

| 范围 | 状态 |
| --- | --- |
| MySQL recover 清理 | 现有 MySQL XA resource 现在会关闭 `XA RECOVER` 返回的 rows，并同时接受 string 与 `[]byte` recover payload，与 MariaDB 已覆盖的 recover 数据形态保持一致。 |
| MySQL/MariaDB XID quoting | MySQL-compatible XA 控制语句现在通过共享 helper 引用 XA XID，执行 `XA START`、`XA END`、`XA PREPARE`、`XA COMMIT` 或 `XA ROLLBACK` 前会转义 XID 内嵌单引号。 |
| PostgreSQL | 保留 prepared transaction XA resource，引用 prepared-transaction XID SQL literal，关闭 `pg_prepared_xacts` recover rows，接受 string 或 `[]byte` recover payload，并已将 `pgconn.PgError` 或文本错误中的 SQLSTATE `42704` / `55000` 分类为 already-ended，用于二阶段幂等状态处理。 |
| MariaDB | 新增 `seata-xa-mariadb`、MariaDB XA resource factory、MySQL-compatible XA 生命周期语句、recover 解析、MariaDB 专属错误分类、XAConn autoCommit、显式事务 commit/rollback、超时、prepare 失败、TC 上报失败、二阶段 held connection 释放和二阶段失败状态分类覆盖、集成测试和用户文档。 |
| SQL XA driver fallback | MySQL、MariaDB 和 PostgreSQL XA 控制语句及 recover 查询现在使用共享 driver prepare fallback；底层 driver 在连接级 exec/query 返回 `driver.ErrSkip` 时，仍可在同一物理连接上 prepare 并执行对应语句。 |
| Oracle | 新增 Oracle `DBMS_XA` XID 映射、生命周期调用、只读 prepare 状态上报、recover 解析与 rows 清理、prepared statement fallback、already-ended 错误分类、DBMS_XA driver transaction state 管理、XAConn autoCommit、显式事务 commit/rollback、超时、prepare 失败、TC 上报失败、二阶段 held connection 释放和二阶段失败状态分类覆盖、单元测试、配置/排查文档，以及 Oracle Free 加 go-ora 外部验证。 |
| XA prepared statement | `XAConn.PrepareContext` 返回的 statement 在执行 `StmtExecContext` / `StmtQueryContext` 时进入与直接 `ExecContext` / `QueryContext` 相同的 XA branch 生命周期；query rows 仍在 `Rows.Close` 时延迟提交 branch。 |
| XA resource 契约测试 | 新增共享 contract coverage，覆盖 supported XA resource factory、resource/classifier 注册、未知 DB type 拒绝、通用 recovery-scan flag 行为、非法 lifecycle flag 拒绝、transaction-timeout 默认值、`IsSameRM` 和 `XAResourcePrepareStatus` 能力边界。 |
| Datasource resource group | `DBResource.GetResourceGroupId` 现在返回当前 RM transaction service group，不再运行时 panic，使 datasource resource 接口与 RM 注册契约保持一致。 |
| 厂商 adapter | 新增 `RegisterSeataXADriver` 和 `SeataDriverDescriptor`，应用可以注册外部 `database/sql/driver.Driver`，无需把厂商 driver 加入 Seata Go 直接依赖；测试覆盖 DBType 必填校验、显式和默认 target name、parse error 传播时不额外拼接原始 DSN、重复注册错误，以及 RM resource ID 中的凭据脱敏，包括 URL、userinfo 前缀、key-value DSN、SSL 敏感字段和非凭据字段保留场景。 |
| 达梦原型 | 新增 `types.DBTypeDM` 和基于 `DBMS_XA` 的 XA resource 原型，覆盖 XID 映射、生命周期调用、只读 prepare 状态上报、recover 解析与 rows 清理、错误分类、XAConn autoCommit、显式事务 commit/rollback、超时、prepare 失败、TC 上报失败、二阶段 held connection 释放和二阶段失败状态分类覆盖、单元测试和文档。 |
| Kingbase 与 Oscar | 已记录扩展方向和待确认问题。Kingbase 优先按 PostgreSQL prepared transaction 路径验证；Oscar 需要先确认公开 Go driver、XA API、recover 和错误码。 |

## 验证命令

提交或更新 PR 前执行：

```bash
git diff --check
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType|IndexConstants' -v
go test ./pkg/protocol/branch ./pkg/protocol/codec -run 'TestBranchStatus|TestBranchReportRequestCodec' -v
go test ./pkg/rm/remoting/grpc -run 'TestGetGrpcRMRemotingInstance|TestGrpcRMRemotingBranchRegisterXAType|TestGrpcRMRemotingBranchReportReadonlyStatus' -v
go test ./pkg/remoting/grpc ./pkg/remoting/processor/client
go test ./pkg/datasource/sql/xa -run 'TestXAResource' -v
go test ./pkg/datasource/sql/xa -run 'Postgres|MariaDB|Oracle|DM' -v
go test ./pkg/datasource/sql/xa -run 'TestPostgresXAConn_Recover|TestPostgresXAErrorClassifierIsAlreadyEnded' -v
go test ./pkg/datasource/sql/xa -run 'TestOracleXAConnRecover|TestOracleXARecoverFallsBackToPrepare|TestDMXAConnRecover|TestDMXARecoverFallsBackToPrepare' -v
go test ./pkg/datasource/sql/xa -run 'TestMysqlXAConn_Recover|TestMariaDBXAConnRecover' -v
go test ./pkg/datasource/sql/xa -run 'TestMysqlXAConn_LifecycleSQLQuotesXID|TestMariaDBXAConnLifecycleSQL' -v
go test ./pkg/datasource/sql/xa -run 'TestPostgresXAConnLifecycleSQLQuotesXID' -v
go test ./pkg/datasource/sql/xa -run 'Test(Mysql|MariaDB|Postgres)XAConn.*FallsBackToPrepare' -v
go test ./pkg/datasource/sql -run 'TestXAConn_PreparedExecContext_AutoCommitCompletesXABranch|TestXAConn_PreparedQueryContext_AutoCommitDefersBranchCommitUntilRowsClose' -v
go test ./pkg/datasource/sql -run 'TestParseResourceID|TestRegisterSeataXADriver|TestXAConn_BeginTx_DBMSXADriverTxHeldUntilPhaseTwo|TestXAConn_BeginTx_DBMSXAReadonlyPrepareReportsReadonlyAndReleases' -v
go test ./pkg/datasource/sql -run 'TestDBResourceGetResourceGroupIdUsesRMConfig|TestDBResourceCheckDbVersionControlsXAConnectionHold|TestXAResourceManager|TestXAConn_BeginTx|TestXAConn_ExecContext|TestXAConn_AutoCommit|TestXATx' -v
go test ./pkg/datasource/sql/xa ./pkg/datasource/sql/types
go test ./pkg/datasource/sql/...
go test ./...
```

具备 MariaDB DSN 时执行集成测试：

```bash
SEATA_GO_TEST_MARIADB_DSN='user:password@tcp(127.0.0.1:3306)/seata_demo?parseTime=true' \
  go test -count=1 -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

当前交付分支已使用本地临时 Docker MariaDB `mariadb:11.4`
（`11.4.13-MariaDB-ubu2404`）验证通过 commit、rollback、recover 清理和重复二阶段回调。
重复回调返回 MariaDB `Error 1397: XAER_NOTA: Unknown XID`，且已完成 XID 不会残留在
`XA RECOVER` 结果中。

当前交付分支也已在 `gvenzl/oracle-free:23-slim-faststart` 的 Oracle Free `23.26.2.0.0`
和 `github.com/sijms/go-ora/v2 v2.9.0` 上完成外部 Oracle 验证。probe 通过
`RegisterSeataXADriver` 注册 `seata-xa-oracle` 风格 wrapper，不再由应用手动调用
`BeginTx`，而是通过 `XAConn.BeginTx` / `XATx.Commit` 完成一阶段 prepare，并通过 held
XA connection 完成二阶段提交。同一环境也确认了 `DBMS_XA.XA_START` 使用 `TMNOFLAGS`、
rollback branch、recover 可见性与清理，以及 readonly prepare 返回 `XA_RDONLY`。可复现的外部
模块流程已记录在 [`xa_oracle_go_ora_validation_zh.md`](./xa_oracle_go_ora_validation_zh.md)，
不把 go-ora 加入项目依赖图。

## 合规检查

| 检查项 | 当前结果 |
| --- | --- |
| 新增依赖 | 当前实现不需要修改 `go.mod` 或 `go.sum`。 |
| 厂商 driver | Oracle 和达梦 driver 由应用通过厂商 adapter API 注入，没有加入项目直接依赖。 |
| Recover rows 清理 | MySQL、MariaDB、PostgreSQL、Oracle 和达梦 recover 读取路径都会关闭 rows；SQL recover 路径接受 driver 以 string 或 bytes 返回 data，DBMS_XA recover 路径已用单元测试覆盖数值、string 和 bytes 字段。 |
| XID SQL literal quoting | MySQL、MariaDB 和 PostgreSQL XA 控制语句在构造 SQL string literal 前会转义 branch XID 内嵌单引号。 |
| Driver prepare fallback | MySQL、MariaDB、PostgreSQL、Oracle 和达梦 XA exec/query 路径在底层 driver 合法返回 `driver.ErrSkip` 时会使用共享 prepare fallback；SQL XA 和 DBMS_XA resource 都有 fallback 测试覆盖。 |
| DBMS_XA driver transaction state | Oracle 和达梦 XA branch 会在 `XA_START` 前打开一层 driver-level transaction，避免 DBMS_XA driver 在 branch 内保持 autoCommit；普通 prepared branch 保留到二阶段，rollback、readonly、failure、close 和 force-close 路径会清理。 |
| Oracle/DM version probing | Oracle 和达梦 resource 初始化跳过 MySQL 风格 `SELECT VERSION()` 探测；它们的连接保活策略不依赖解析服务端版本，部分 driver 也不接受该查询。 |
| XA 连接保活策略 | `DBResource.checkDbVersion` 统一决定是否保活：MySQL 8.0.29 之前版本、MariaDB、Oracle 和达梦保留 prepared 连接；MySQL 8.0.29+ 和 PostgreSQL 不再仅因 DBType 已知而强制保活。 |
| Prepared statement XA 生命周期 | 带 context 的 prepared statement 在执行时进入 XA branch 生命周期，而不是在 prepare 时提交；prepared query rows 继续保持 close-time branch commit 行为。 |
| XA resource 契约 | MySQL、PostgreSQL、MariaDB、Oracle 和达梦均有共享 factory/resource/classifier、recovery-scan 和非法 lifecycle flag 契约测试覆盖；unsupported DB type 保持默认 no-op classifier，并返回明确 resource factory 错误。 |
| Datasource resource group | `DBResource.GetResourceGroupId` 跟随 `rm.GetRmConfig().TxServiceGroup`，`rm.Resource` 必需方法可安全调用，并与 RM 注册请求使用的 transaction service group 保持一致。 |
| PostgreSQL 二阶段幂等 | PostgreSQL SQLSTATE `42704` 和 `55000` 已分类为 already-ended 错误，重复或延迟二阶段失败时可按 RM 契约缓存 committed/rollbacked 状态。 |
| 只读 prepare 状态 | 普通协议枚举已补 `BranchStatusPhaseoneReadonly = 13`，与已有 gRPC `PhaseOne_RDONLY` 值对齐；普通 codec 和 gRPC branch-report request 覆盖均保留 readonly 状态。 |
| gRPC XA 分支类型 | `BranchTypeProto` 已显式包含 `XA = 3`，与 XA 分支注册、上报、提交和回滚消息使用的普通协议 `BranchTypeXA` 值对齐；gRPC branch-register、branch-report 和 branch-end processor 测试均覆盖 XA 映射。 |
| License header | 新增 Go 和 Markdown 文件均包含 Apache Software Foundation license header。 |
| 生成文件 | `dbtype_string.go` 已随 DB type 测试同步更新。 |
| 敏感信息 | 文档示例使用占位值，不应包含真实 DSN、密码、wallet 或私有部署信息；生成 RM resource ID 时会在注册前去除 URL userinfo、MySQL/vendor 风格的 `user:password@` 前缀，以及 `user=`、`password=`、`sslpassword=`、`passfile=` 和 SSL key/certificate 路径等 key-value 字段，同时保留用于稳定识别 resource 的非凭据 key-value 字段。 |

## 建议 PR 拆分

| PR | 范围 | 说明 |
| --- | --- | --- |
| PR 1 | Java/Go 对比、XA 契约说明和 MariaDB resource | 先让第一个新增数据库支持可独立评审，并包含 MariaDB 集成测试路径。 |
| PR 2 | Oracle XID helper 和 `DBMS_XA` 生命周期 | 聚焦 XID 编码、PL/SQL block 构造和生命周期映射。 |
| PR 3 | Oracle recover、classifier、fallback 和文档 | 把恢复语义、driver fallback 和用户配置说明放在一起评审。 |
| PR 4 | 厂商 XA adapter 注册入口 | 单独评审外部 driver 注入的公开 API。 |
| PR 5 | 达梦原型和国产数据库扩展说明 | 达梦在真实库验证前按 prototype 呈现。 |

如果维护者倾向更少 PR，可以重新 squash，但 PR 描述仍应保留这些边界。

## 剩余验证

| 优先级 | 事项 | 原因 |
| --- | --- | --- |
| P1 | 更完整的 Oracle 验证矩阵 | Oracle Free 加 go-ora 已确认 wrapper 路径、命名绑定、输出参数、权限、recover、readonly prepare 和 held connection 二阶段提交；生产支持口径前仍建议补充其他 Oracle edition、driver、重复回调、branch missing 错误和连接断开场景。 |
| P0 | 达梦 driver 与许可证验证 | 当前原型刻意不引入直接依赖，官方 driver 来源、版本、许可证和再分发条款明确前不能声明生产支持。必查项见 [`xa_dm_validation_zh.md`](./xa_dm_validation_zh.md)。 |
| P1 | 达梦真实库验证 | `DBMS_XA` 可用性、`XA_COMPATIBLE_MODE`、recover 形态和真实错误码需要实测。生命周期与错误语义清单见 [`xa_dm_validation_zh.md`](./xa_dm_validation_zh.md)。 |
| P2 | Kingbase 原型决策 | 基于 PostgreSQL prepared transaction 的原型可行性较高，但应等待 driver 和兼容性确认。 |
| P2 | Oscar 后续跟进 | 只有在公开 Go driver、XA API、recover 和错误码证据明确后再新增代码。 |

## 支持等级表述

release notes 和 PR 描述建议使用准确措辞：

- MariaDB：具备单元测试、文档，并已在 MariaDB 11.4.13 上验证 lifecycle、recover 清理和重复二阶段回调的 XA resource 支持。
- PostgreSQL：已有 prepared transaction XA resource，并补充 XID SQL literal quoting、recover rows 清理和基于 SQLSTATE 的二阶段 already-ended 分类。
- Oracle：具备 mock 覆盖、配置文档，并已通过 Oracle Free 加 go-ora 外部验证 wrapper 一阶段 prepare、二阶段提交、rollback、recover 和 readonly prepare；生产支持口径前仍建议补充更完整的 driver / edition 验证。
- 厂商 adapter：用于包装外部 driver 的公开扩展 API，不引入直接厂商依赖。
- 达梦：具备只读 prepare 状态传播 mock 覆盖；真实 driver 许可证、兼容模式、recover 输出和错误码验证前，只标记为 prototype resource。
