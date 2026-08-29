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

# 达梦 XA 原型使用指南

Seata Go 通过 `DBMS_XA` PL/SQL 包和 `types.DBTypeDM` 提供达梦 XA resource 原型。该原型不把达梦
driver 作为 Seata Go 的直接依赖引入。应用需要先通过厂商 adapter API 注册选定的达梦
`database/sql/driver.Driver`，再打开数据库连接。

```go
import (
	stdsql "database/sql"
	"database/sql/driver"

	seatasql "seata.apache.org/seata-go/v2/pkg/datasource/sql"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

func registerDMXA(dmDriver driver.Driver) error {
	return seatasql.RegisterSeataXADriver("seata-xa-dm", seatasql.SeataDriverDescriptor{
		DBType:     types.DBTypeDM,
		Target:     dmDriver,
		TargetName: "dm",
		ParseDBName: func(dsn string) (string, error) {
			return parseDMDBName(dsn)
		},
	})
}

db, err := stdsql.Open("seata-xa-dm", dsn)
```

`parseDMDBName` 应返回应用用于标识 Seata resource 的达梦 schema、数据库名或服务名。不要在日志中输出完整
DSN。

## XA 映射

| Seata Go 方法 | 达梦调用 |
| --- | --- |
| `Start` | `DBMS_XA.XA_START` |
| `End` | `DBMS_XA.XA_END` |
| `XAPrepare` | `DBMS_XA.XA_PREPARE` |
| `Commit` | `DBMS_XA.XA_COMMIT` |
| `Rollback` | `DBMS_XA.XA_ROLLBACK` |
| `Recover` | `TABLE(DBMS_XA.XA_RECOVER())` |
| `Forget` | `DBMS_XA.XA_FORGET` |

原型把 Seata branch XID 映射到 `DBMS_XA_XID(formatid, gtrid, bqual)`：

- `formatid`：固定为 `9752`，对齐 Seata Java `XABranchXid`。
- `gtrid`：Seata 全局 XID 字节。
- `bqual`：Seata branch qualifier 字节，编码为 `-<branchId>`。
- `gtrid` 和 `bqual` 都必须满足 `RAW(64)` 限制。

## 使用要求

- 使用支持 `DBMS_XA` 的达梦版本和兼容模式。
- 达梦 Go driver 需要能通过 `ExecerContext`，或 `PrepareContext` 加 `StmtExecContext` 执行 PL/SQL block。
- 如果应用需要把 `XA_RDONLY` prepare 分支上报为 `BranchStatusPhaseoneReadonly`，达梦 Go driver 还需要支持 `database/sql.Out` 命名输出参数。
- 达梦 Go driver 需要能通过 `QueryerContext`，或 `PrepareContext` 加 `StmtQueryContext` 执行恢复扫描。
- 在把 driver 加入任何 Apache 仓库依赖前，必须确认来源、许可证和再分发条款。
- 正式使用前，需要在真实达梦数据库上验证 `XA_RECOVER`、branch missing、重复二阶段回调和非法 XID 错误。

真实 driver 和真实数据库验证项见 [达梦 XA 验证清单](./xa_dm_validation_zh.md)。

## 本地测试

达梦原型测试使用 mock `driver.Conn`，不需要真实达梦数据库：

```bash
go test ./pkg/datasource/sql/xa -run DM -v
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType' -v
```

完整 SQL 包测试：

```bash
go test ./pkg/datasource/sql/...
```

## 已知限制

| 限制 | 状态 |
| --- | --- |
| 真实库验证 | 当前仓库尚未完成。 |
| driver 依赖 | 未加入 `go.mod`，由应用外部注册 driver。 |
| 兼容模式 | 需要在目标达梦部署上确认 `DBMS_XA` 是否可用；达梦文档说明 MySQL 兼容 XA 模式不使用 `DBMS_XA` 包。 |
| PL/SQL 与 recover 形态 | 当前原型使用 Oracle-like anonymous block 和 `TABLE(DBMS_XA.XA_RECOVER())` 查询形态，需要结合选定达梦版本和 driver 实测。 |
| 只读 prepare 上报 | Go 侧通过输出参数读取 `DBMS_XA.XA_PREPARE` 返回码，并在返回 `XA_RDONLY` 时上报 `BranchStatusPhaseoneReadonly`；该绑定行为仍需真实 driver 验证。 |
| 错误语义 | 捕获真实错误前，仅把通用 `XAER_NOTA` 文本和数字 `-4` 识别为 already-ended。 |
| 表元数据 | 当前原型没有实现达梦 table metadata cache。 |
