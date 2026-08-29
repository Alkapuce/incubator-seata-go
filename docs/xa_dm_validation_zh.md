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

# 达梦 XA 验证清单

本文记录达梦 XA resource 从 prototype 口径推进到 supported 口径前必须完成的验证工作。当前实现不把
达梦 driver 作为直接依赖加入 Seata Go。应用需要通过 `RegisterSeataXADriver` 注入选定的 driver。

## 来源检查点

当前清单基于达梦公开文档，正式声明生产支持前需要重新核对：

- [DM Go 编程指南](https://eco.dameng.com/document/dm/zh-cn/pm/go-rogramming-guide.html)
  给出 `driverName := "dm"` 和 Go driver 配置方式。
- [DM GO 接口指南](https://eco.dameng.com/document/dm/zh-cn/app-dev/go_dm.html)
  说明达梦 GO 驱动位于数据库安装目录的 `drivers/go` 下，解压后再复制到 Go 源码路径。
- 公开 Git 镜像或二次打包 driver 不能直接作为 Apache 依赖依据，除非能确认官方所有权、版本、许可证和再分发条款。
- [DM XA 编程指南](https://eco.dameng.com/document/dm/zh-cn/pm/xa-rogramming-guide.html)
  说明 `XA_COMPATIBLE_MODE=2` 走 MySQL XA 兼容策略，不支持 `DBMS_XA` 包。

## Driver 边界

新增依赖或声明生产支持前，先验证选定 driver：

- 确认 driver 来源、版本、许可证和再分发条款。
- 除非维护者明确认可其他来源，优先使用达梦数据库安装包内随附的官方 driver。
- 记录 `database/sql` 使用的 driver name、import package path 和测试时的 Go 版本。
- 确认 driver 实现原型需要的接口：`driver.Conn`、`driver.ExecerContext` 或
  `driver.ConnPrepareContext` 加 `driver.StmtExecContext`、`driver.QueryerContext` 或
  `driver.StmtQueryContext`，以及 `driver.ConnBeginTx`。
- 确认 `database/sql.Out` 命名输出参数是否能用于 `DBMS_XA.XA_PREPARE`。

## 数据库准备

当前原型预期达梦提供 Oracle-like `DBMS_XA` API 和 `DBMS_XA_XID(formatid, gtrid, bqual)`
结构。需要在目标达梦部署上验证：

- 达梦 edition 和版本。
- 当前部署是否启用 `DBMS_XA`。
- `XA_COMPATIBLE_MODE` 的取值，以及它对 `DBMS_XA` 可用性的影响。
- 应用用户需要的授权。
- `TABLE(DBMS_XA.XA_RECOVER())` 是否按原型 parser 需要的形态返回 `formatid`、`gtrid` 和
  `bqual`。
- 应用连接关闭后，prepared branch 是否仍可被 recover 并完成二阶段。

不要把真实密码、私有主机、wallet 路径或完整 DSN 写入仓库文档。

## 生命周期验证

使用一次性 schema 和表。至少验证：

1. `XA_START` 使用 `TMNOFLAGS`。
2. XA branch 内执行业务 insert。
3. `XA_END` 使用 `TMSUCCESS`。
4. `XA_PREPARE` 返回 `XA_OK`。
5. `XA_RECOVER` 能看到 prepared branch。
6. `XA_COMMIT` 让业务行可见，并从 recovery 结果中移除 branch。
7. 另一条 prepared branch 执行 `XA_ROLLBACK`。
8. 如果 driver 和数据库支持输出参数路径，验证 read-only branch 的 `XA_PREPARE` 返回
   `XA_RDONLY`。

## 错误语义

记录选定 driver 和数据库返回的原始错误：

- 重复二阶段 commit。
- 重复二阶段 rollback。
- commit 时 branch missing。
- rollback 时 branch missing。
- 非法 XID 或超长 XID 字段。
- `DBMS_XA` 权限不足。
- prepare 前后连接断开。

当前 classifier 只把通用 `XAER_NOTA` 文本和数字 `DBMS_XA` 返回码 `-4` 视为 already-ended。只有真实
达梦错误证明需要额外模式时，才扩展 classifier。

## 支持口径

清单完成前，使用以下表述：

```text
Dameng is provided as a DBMS_XA-based prototype. It has mock coverage for the
Seata Go lifecycle, readonly prepare status propagation, recovery parsing, and
phase-two cleanup, but it still requires real-driver license, compatibility
mode, recovery output, and error-code validation before production support.
```

