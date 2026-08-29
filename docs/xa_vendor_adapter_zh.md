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

# 厂商 XA 适配指南

Seata Go 可以把外部 `database/sql/driver.Driver` 包装成 Seata XA driver，不需要把厂商 driver 作为项目直接依赖引入。

应用初始化阶段先注册厂商 driver，必须早于 `sql.Open` 使用新的 driver 名称：

```go
import (
	stdsql "database/sql"

	seatasql "seata.apache.org/seata-go/v2/pkg/datasource/sql"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/datasource"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

err := seatasql.RegisterSeataXADriver("seata-xa-vendor", seatasql.SeataDriverDescriptor{
	DBType:     types.DBTypeOracle,
	Target:     vendorDriver,
	TargetName: "vendor-driver-name",
	ParseDBName: func(dsn string) (string, error) {
		return parseVendorDBName(dsn)
	},
	NewTableMetaCache: func(db *stdsql.DB, dbName string) datasource.TableMetaCache {
		return vendorTableMetaCache(db, dbName)
	},
})
```

之后通过注册出的 Seata XA driver 名称打开数据库：

```go
db, err := stdsql.Open("seata-xa-vendor", dsn)
```

## Descriptor 字段

| 字段 | 是否必填 | 用途 |
| --- | --- | --- |
| `DBType` | 是 | 选择 XA resource factory 和数据库专属行为。 |
| `Target` | 是 | 底层厂商 `driver.Driver`。 |
| `TargetName` | 否 | wrapped connector 暴露的目标 driver 名；默认使用 `DBType.String()`。 |
| `ParseDBName` | 是 | 从厂商 DSN 中提取数据库名或 schema 名。 |
| `NewTableMetaCache` | 否 | 厂商适配需要表元数据时注册；纯 XA 适配可以留空。 |

## 适配清单

- 为厂商 `DBType` 注册或实现 XA resource factory。
- 提供稳定的 Seata driver 名称，例如 `seata-xa-vendor`。
- 从厂商 DSN 中解析数据库名，避免在日志中输出密码。
- 确保由 DSN 派生的 RM resource ID 保持稳定，但不携带 URL userinfo、MySQL/vendor 风格的 `user:password@`，或 `user=`、`password=`、`sslpassword=`、`passfile=` 和 SSL key/certificate 路径等 key-value 凭据。
- 增加单元测试，证明 wrapper 返回的 `XAConn` 具有预期 DB type、数据库名、DBType 必填校验、显式或默认 target driver name、parse error 传播时不额外拼接原始 DSN、重复注册错误，以及 resource ID 凭据脱敏，包括 URL、userinfo 前缀和 key-value DSN 形式。
- 增加集成测试，覆盖 start、end、prepare、commit、rollback、recover、branch missing 和重复二阶段回调。
- 文档记录 driver 版本、数据库版本、权限、恢复行为和不支持的 XA flag。
