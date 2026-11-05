# 🔌 Grafana OpenObserve 数据源插件

<div align="center">

![GitHub](https://img.shields.io/github/license/LinPr/grafana-openobserve-datasource?style=flat-square)
![Grafana](https://img.shields.io/badge/Grafana-10.4%2B-orange?style=flat-square&logo=grafana)
![OpenObserve](https://img.shields.io/badge/OpenObserve-Supported-blue?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat-square&logo=go)
![TypeScript](https://img.shields.io/badge/TypeScript-5.5.4+-3178C6?style=flat-square&logo=typescript)
![Node.js](https://img.shields.io/badge/Node.js-22+-339933?style=flat-square&logo=node.js)

[English](README.md) · **简体中文**

</div>

---

## 🛰️ 简介

一个面向 [OpenObserve](https://github.com/openobserve/openobserve) 的 Grafana 数据源插件：
用 SQL 查询 **日志（logs）**、**指标（metrics）** 和 **链路（traces）**，或者用 **PromQL**
访问 OpenObserve 的 Prometheus 兼容接口，把结果直接变成 Grafana 的 panel、dashboard、
变量和 annotation。

本插件是非官方实现。官方插件在
[这里](https://github.com/openobserve/openobserve-grafana-plugin)；本项目的目标是「用起来像
Grafana 原生数据源」，而不是把 OpenObserve 的原始 API 暴露给用户。

插件标识为 `linpr-openobserve-datasource`，dashboard、provisioning 文件和
`grafana-cli` 都通过它来引用插件：

| 项目              | 值                                             |
| ----------------- | ---------------------------------------------- |
| 数据源类型        | `linpr-openobserve-datasource`                 |
| 后端可执行文件    | `linpr-grafana-openobserve-datasource`         |
| 容器镜像          | `ghcr.io/linpr/grafana-openobserve-datasource` |
| 最低 Grafana 版本 | 10.4.0                                         |

> 类型名的第一段是发布者的 Grafana Cloud 组织 slug（`linpr`），这是插件市场的硬性要求。
> **二进制文件**仍保留 `grafana-` 前缀，这样在进程列表里或看到一个残留文件时，仍然能认出
> 它属于 Grafana 插件。

> **下面所有截图都来自真实的 Grafana 12.2 + OpenObserve 环境，不是效果图。**

---

## 📸 截图

### 功能演示

一段在真实 Grafana + OpenObserve 上录制的演示，顺序就是配置这个插件的顺序：数据源设置、
日志（展开一行查看原始列）、用 builder 查指标、PromQL、trace 列表、一条真实的 9 span
瀑布图、服务依赖图，最后是一个混合了日志、时序、环形图、柱状图和 trace 表格的 dashboard。

![功能演示](doc/screenshot/tour.gif)

> GIF 是 2.4 MB，每个画面停留半秒。更清晰的 [WebP 版本](doc/screenshot/tour.webp)
> 只有 0.35 MB，分辨率更高；每个画面在下方也都有静态图。

### Explore 中的日志

日志行按 level 上色，详情面板里每个原始列都变成 attribute：

| Explore 中的日志                                     | 查询编辑器                                     |
| ---------------------------------------------------- | ---------------------------------------------- |
| ![Explore 中的日志](doc/screenshot/explore_logs.png) | ![查询编辑器](doc/screenshot/query_editor.png) |

编辑器第一行是 stream type、query mode（SQL / PromQL）和 Builder / Code 切换，
第二行是 stream，之后是 builder 的 Raw / Aggregate 开关。Limit 放在最后，因为它不参与
决定查询形状。Builder 和 Code 是相互独立的，所以在 Builder 里点什么都不会覆盖你手写的语句。

### 用 SQL 查指标，自动生成序列标签

`GROUP BY service_name` 直接变成按 service 分组的多条序列——不需要 transform，也不需要
`AS` 之类的 SQL 技巧。同样的指标流也能通过 OpenObserve 的 Prometheus 兼容接口查询，
所以 `rate()`、`histogram_quantile()` 和 label matcher 都能用：

| 用 SQL 查指标                                            | PromQL                                       |
| -------------------------------------------------------- | -------------------------------------------- |
| ![用 SQL 查指标](doc/screenshot/explore_metrics_sql.png) | ![PromQL](doc/screenshot/explore_promql.png) |

### 链路：三种查询方式

trace 编辑器对齐了 Jaeger 的三种模式，SQL 全部由后端生成。**Search** 列出符合筛选条件的
trace，点 trace id 就能在 Grafana 里打开这条 trace；**TraceID** 加载一条 trace 的全部
span，由 Explore 画原生瀑布图；**Graph** 从 span 的父子关系推导服务间的调用次数并画成节点图，
被调用方取的是 OpenObserve 推断出的 service name，所以 client span 会归到它调用的系统上，
而不是归到调用方。

| Search                                         | TraceID（瀑布图）                                   | Graph                                         |
| ---------------------------------------------- | --------------------------------------------------- | --------------------------------------------- |
| ![Trace 搜索](doc/screenshot/trace_search.png) | ![Trace 瀑布图](doc/screenshot/trace_waterfall.png) | ![服务依赖图](doc/screenshot/trace_graph.png) |

### 一个什么都有的 dashboard

日志、stat、饼图、堆叠时序、PromQL 和 trace 表格，全部来自同一个数据源：

![Dashboard](doc/screenshot/dashboard_logs_metrics.png)

### 模板变量

一个变量就是一条 SQL，第一列就是变量的值。多选值会被正确地加引号拼进 `IN (...)`：

| 带变量的 dashboard                                        | 变量查询编辑器                                              |
| --------------------------------------------------------- | ----------------------------------------------------------- |
| ![Dashboard 变量](doc/screenshot/dashboard_variables.png) | ![变量查询编辑器](doc/screenshot/variable_query_editor.png) |

### Ad-hoc 过滤器

过滤器的 key 就是该 panel 所查 stream 的真实列，选中的过滤条件由服务端注入成 SQL 谓词——
下图里的 `severity = 'ERROR'` 在不改动查询语句的前提下收窄了日志 panel：

![Ad-hoc 过滤器](doc/screenshot/dashboard_adhoc_filter.png)

### 数据源设置

连接、认证（basic auth 或 token）、TLS、查询行数限制和列映射：

| Basic 认证                                      | Token 认证                                            |
| ----------------------------------------------- | ----------------------------------------------------- |
| ![数据源设置](doc/screenshot/config_editor.png) | ![Token 认证](doc/screenshot/config_editor_token.png) |

### Panel 一览

|                                                                             |                                                           |
| --------------------------------------------------------------------------- | --------------------------------------------------------- |
| **日志面板**<br>![日志面板](doc/screenshot/log.png)                         | **柱状图**<br>![柱状图](doc/screenshot/log_bar_chart.png) |
| **日志数量直方图**<br>![日志数量](doc/screenshot/log_count_time_series.png) | **饼图**<br>![饼图](doc/screenshot/metrics_pie_chart.png) |
| **内存占用**<br>![内存占用](doc/screenshot/memory_usage.png)                |                                                           |

---

## ✨ 主要特性

### 查询

- **多种 stream 类型** —— 一个数据源同时覆盖 `logs`、`metrics`、`traces`
- **SQL 编辑器**，支持 stream 名、列名、关键字和函数补全
- **PromQL 模式**，走 `/api/{org}/prometheus`，指标 panel 的体验和原生 Prometheus 数据源一致
- **结果格式**：`Auto`、`Logs`、`Table`、`Time series`、`Trace`、`Annotations`
- **SSE 流式搜索**（`_search_stream`），部署端返回普通 JSON 时自动降级
- **行数限制**：单查询 limit → SQL `LIMIT` → 数据源默认值，最终都受配置的最大值约束

### Grafana 能直接理解的帧

- **自动识别时间列** —— `_timestamp`（微秒）、`start_time`（纳秒）和 ISO 字符串都会变成
  Grafana 的 `time` 字段，做时序图不需要写 `AS gf_time`，也不需要 transform
- **序列标签** —— 分组列变成 frame 的 label，也就是图例
- **原生日志帧** —— `time`、`body`、`level`、`labels`，外加每个原始列作为 attribute，
  所以 Explore 能显示 level 颜色和完整详情
- **原生 trace 帧** —— span 会带上 `traceID`、`spanID`、`parentSpanID`、`serviceName` 等字段，
  瀑布图由 Grafana 自己绘制
- **按 schema 定型** —— 数字、布尔、时间、JSON 的类型来自 stream schema，再用实际值做交叉校验，
  计算出来的别名也不会拿到错误类型
- **Annotation** —— `Annotations` 格式把行转成 dashboard 上的 annotation

### 与 Grafana 的集成

- **Ad-hoc 过滤器**在服务端注入成真正的 SQL 谓词：多个条件按 AND 合并，已有的 `OR` 链会先加
  括号（保持原意），值会被转义，`number(123)` 保持数字，`=~` / `!~` 翻译成 OpenObserve 的
  `~` / `!~`
- **模板变量**支持能识别引号的 SQL 拼接和多值选择
- **Tag keys / tag values** 来自真实 stream schema，所以 ad-hoc 变量的补全对的就是你的列
- **看得懂的报错** —— OpenObserve 的错误体会被透出（stream 不存在、认证失败、组织不存在），
  因为 stream 类型不匹配导致空结果时也会自己解释，而不是留一个空白 panel

### 连接

- Basic auth（用户名 + 密码）**或** token，token 原样放进 `Authorization` 头
- 单查询超时可配置，支持取消传播，支持跳过 TLS 校验
- 健康检查会报出连到的组织和 stream 数量

---

## 📖 文档

- [**功能说明**](doc/features.md) —— 查询模式、结果格式、列映射、变量、ad-hoc 过滤器、
  annotation，以及日志 / 指标 / 链路的 SQL cookbook
- [**开发指南**](doc/develop.md) —— 如何构建、运行和测试插件
- [**更新日志**](CHANGELOG.md) —— 各版本的变化
- [**English README**](README.md) —— 英文文档

---

## 🚀 安装

按你的 Grafana 情况选一种：

1. **插件市场** —— 插件上架并签名后，可以从 Grafana Cloud 安装，或用
   `grafana cli plugins install linpr-openobserve-datasource`。
2. **Grafana CLI 直接从 GitHub 装** —— Release 里的包没有签名，但只要用
   `--pluginUrl` 指定压缩包地址，`grafana cli` 照样能装，省掉手动解压这一步
   （见 [Grafana CLI](#grafana-cli)）。
3. **Release 压缩包** —— 从
   [Releases](https://github.com/LinPr/grafana-openobserve-datasource/releases) 下载对应平台的
   压缩包，解压到插件目录，见下文。
4. **容器镜像** —— 从 `ghcr.io/linpr/grafana-openobserve-datasource` 里把插件拷出来
   （见 [Docker](#docker)）。

每个 release 都会发布**每个平台一个压缩包**，外加一个包含全部平台的通用包。
压缩包按插件 id 命名，所以解压出来的目录叫 `linpr-openobserve-datasource`，
和你下载的文件同名，也是 Grafana 认的名字：

| 压缩包                                                     | 内容                                        |
| ---------------------------------------------------------- | ------------------------------------------- |
| `linpr-openobserve-datasource-<version>.zip`               | 全部平台，Grafana 插件市场要的就是这个      |
| `linpr-openobserve-datasource-<version>.linux-amd64.zip`   | Linux x86-64                                |
| `linpr-openobserve-datasource-<version>.linux-arm64.zip`   | Linux ARM64（Ampere、Graviton、树莓派 4/5） |
| `linpr-openobserve-datasource-<version>.linux-arm.zip`     | Linux ARMv7（32 位）                        |
| `linpr-openobserve-datasource-<version>.darwin-amd64.zip`  | macOS Intel                                 |
| `linpr-openobserve-datasource-<version>.darwin-arm64.zip`  | macOS Apple Silicon                         |
| `linpr-openobserve-datasource-<version>.windows-amd64.zip` | Windows x86-64                              |

`checksums.txt` 里是每个文件的 SHA-1。

### Grafana CLI

`grafana cli` 默认只能装市场里已签名的插件。但加上 `--pluginUrl` 之后，它会直接下载并解压
任意地址的压缩包 —— 这是安装 Release 构建最省事的方式，不用自己往插件目录里拷：

```bash
grafana cli \
  --pluginsDir /var/lib/grafana/plugins \
  --pluginUrl https://github.com/LinPr/grafana-openobserve-datasource/releases/download/v<version>/linpr-openobserve-datasource-<version>.zip \
  plugins install linpr-openobserve-datasource
```

成功时会打印 `✔ Downloaded and extracted` 加上插件 id 和版本号。注意地址要用**压缩包**的
（`releases/download/<tag>/<文件名>`），不要用 release 页面或 tag 页面 —— 给它一个 HTML
页面就会报出那个很容易误导人的 `Could not process plugin archive: Error reading file`。

`--pluginUrl` 只是绕过了签名校验；Grafana 依然不会加载未签名的插件，除非你显式允许，
所以重启时要带上这个开关：

```bash
export GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource
```

### Linux / macOS

```bash
# 换成你平台对应的压缩包，然后：
sudo unzip linpr-openobserve-datasource-<version>.linux-amd64.zip \
  -d /var/lib/grafana/plugins/
export GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource
```

然后重启 Grafana。

### Docker

多架构镜像同时提供 `linux/amd64` 和 `linux/arm64`，`docker pull` 会自动选对：

```bash
docker run -d --name grafana -p 3000:3000 \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource \
  -v grafana-plugins:/var/lib/grafana/plugins \
  grafana/grafana

# 从已发布的镜像里把插件拷进去
docker run --rm -v grafana-plugins:/var/lib/grafana/plugins \
  ghcr.io/linpr/grafana-openobserve-datasource:latest \
  cp -a /var/lib/grafana/plugins/linpr-openobserve-datasource /var/lib/grafana/plugins/
```

也可以直接挂载，省掉倒腾 volume 的步骤：

```bash
docker run -d --name grafana -p 3000:3000 \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource \
  --mount type=bind,source=/opt/grafana-plugins/linpr-openobserve-datasource,target=/var/lib/grafana/plugins/linpr-openobserve-datasource \
  grafana/grafana
```

### Provisioning

批量部署最快的方式是用 provisioning 文件。
[`provisioning/datasources/datasources.yml`](provisioning/datasources/datasources.yml)
是一个完整示例：

```yaml
apiVersion: 1

datasources:
  - name: 'OpenObserve'
    type: 'linpr-openobserve-datasource'
    access: proxy
    url: 'http://localhost:8000'
    user: 'user'
    jsonData:
      organization: 'default' # 旧的 `database` 键仍然兼容
      authMethod: 'basic' # 或者 'token'
      timeoutSeconds: 60
      defaultLimit: 500
      maxLimit: 10000
      enablePrometheus: true
    secureJsonData:
      password: 'password' # authMethod 为 'token' 时改填 token
```

迁移已有数据源时请保留原来的 `uid`，这样引用它的 dashboard 才不会失效。

### 从 2.3.x 升级

2.4.0 把数据源类型从 `grafana-openobserve-datasource` 改成了
`linpr-openobserve-datasource`，因为插件市场拒绝旧 id（`grafana` 属于 Grafana Labs 的组织）。
影响如下：

- 指向旧类型的 dashboard 和 provisioning 数据源将无法解析。请在受影响的 panel 上重新选择
  数据源，或修改 provisioning 文件里的 `type:`。保留 `uid`，panel 就能继续工作。
- 后端二进制从 `gpx_open_observe` 变成了 `linpr-grafana-openobserve-datasource`。这个名字
  不需要任何人知道——Grafana 是从 `plugin.json` 里读的——但手动安装时必须整个插件目录一起
  替换，只换二进制不行。
- 其他都不变：查询、设置和已存的 JSON 都是兼容的。

---

## 🔍 使用

### 1. 配置数据源

把 URL 指向你的 OpenObserve 服务器，填好组织（通常是 `default`），选择认证方式。见上文的
[数据源设置](doc/screenshot/config_editor.png)。**Save & test** 会报出连到的组织以及找到的
stream 数量，所以 URL 写错、组织写错或认证失败，在你写第一条查询之前就能发现。

### 2. 查日志

```sql
SELECT * FROM "my_logs" LIMIT 500
```

对日志流做 `SELECT *` 会渲染成日志行，带 level 颜色，每个原始列都作为 attribute。没有 Format
开关要设：帧的形状由 stream 类型和语句本身决定。

### 3. 查指标

```sql
SELECT histogram(_timestamp, '1 minute') AS ts,
       avg(value) AS value,
       service_name
FROM "go_goroutine_count"
GROUP BY ts, service_name
ORDER BY ts
```

时间列加上数字就渲染成时序，每个 `service_name` 一条线，不需要任何 transform。

或者把 **Query mode** 切到 `PromQL`，直接写 `go_goroutine_count`。

### 4. 查链路

选择 `traces` stream 类型，挑一个 trace stream，然后选查询方式：

- **Search** 列出符合筛选条件的 trace。
- **TraceID** 把一条 trace 渲染成瀑布图。
- **Graph** 画服务依赖图。

### 5. Builder 还是 Code

和 Grafana 其他数据源一样，编辑器有两种模式：

- **Builder** 接收结构化输入——stream、Raw 或 Aggregate，然后是 function、field、interval
  和 group by——SQL 由后端推导。有一个只读预览显示将要执行的语句。它不会往 Code 编辑器里写
  任何东西，所以不想手写 SQL 时用这个模式。
- **Code** 就是你写的语句，带 stream 和列的补全。

切换模式不会丢东西：结构化字段和语句是分开存的，来回切换两边都保留。已保存的查询如果已经有
SQL，会直接以 Code 打开，因为那条语句是手写的。

### 6. Limit

**Limit** 字段和 SQL 里的 `LIMIT` 是同步的：改一个另一个跟着变，所以 panel 和语句永远不会
在行数上打架。

### 7. 变量和 ad-hoc 过滤器

模板变量就是一条 SQL，第一列是值：

```sql
SELECT DISTINCT service_name AS "value" FROM "my_logs"
```

然后在查询里用 `$service_name`——插件会正确地给多选值加引号，所以
`WHERE service_name IN ($service_name)` 直接就能用。

在 dashboard 上，添加一个类型为 `linpr-openobserve-datasource` 的 **ad-hoc 过滤器**变量：
它的 key 来自 stream schema，选中的条件会变成所有使用该数据源的 panel 上额外的 `WHERE` 谓词。

### 8. Annotation

新增一个 annotation 查询，选择 `Annotations` 格式，返回一个时间列加上你想看到的文本：

```sql
SELECT _timestamp AS time, body AS text
FROM "my_logs"
WHERE level = 'error'
```

时间列作为 annotation 的时间戳，body/text 列作为标题和描述，于是日志流里的发布、告警和故障
就会出现在 dashboard 的时间轴上。

---

## ✅ 兼容性

每个受支持的大版本，都用它的最后一个 patch 版本对同一个在线 OpenObserve 实例做过测试：

| Grafana | 健康检查 | 日志（builder） | 指标（builder） | PromQL | Trace 搜索 | Trace 图 | UI  |
| ------- | :------: | :-------------: | :-------------: | :----: | :--------: | :------: | :-: |
| 10.4.19 |    ✅    |       ✅        |       ✅        |   ✅   |     ✅     |    ✅    | ✅  |
| 11.6.16 |    ✅    |       ✅        |       ✅        |   ✅   |     ✅     |    ✅    | ✅  |
| 12.4.14 |    ✅    |       ✅        |       ✅        |   ✅   |     ✅     |    ✅    | ✅  |
| 13.3.0  |    ✅    |       ✅        |       ✅        |   ✅   |     ✅     |    ✅    | ✅  |

CI 还会用同一套 Playwright 用例跑 `nightly` 镜像，所以未来的 Grafana 一旦破坏兼容性，在发版前
就会被发现。

「UI」指 trace 编辑器在 Explore 里渲染出三种查询方式、Attributes 字段和一个有数据的
结果表，且没有页面错误和控制台错误。

Grafana 在运行时提供 `@grafana/ui`、`@grafana/data` 和 `@grafana/runtime`，所以插件只使用
最老支持版本里就存在的 API。例如 `Combobox` 是 12.2 才加的，因此刻意没有使用，继续用已废弃的
`Select`。

## 🛠️ 开发

环境要求：

- **Node.js** 22 或更新
- **Go** 1.27.1 或更新（`go.mod` 里锁定了版本，CI 和本地保持一致）
- **OpenObserve** 0.14.7 或更新
- **Docker**，用于开发时运行本地 Grafana

```bash
npm install
npm run typecheck        # tsc --noEmit
npm run lint             # eslint
npm run test:ci          # jest
npm run build            # webpack

go build ./...
go test ./pkg/...
```

要在真实 Grafana 里跑插件，设置
`GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource` 后执行
`docker compose up --build`；compose 文件把 `dist/` 挂进容器，所以 `npm run build`
（或用 `npm run dev` 进入 watch 模式）就能看到前端改动。

上面那些截图所用的在线验证脚本在 [tests/live](tests/live/README.md)，它们会对真实的
OpenObserve 跑真实查询，并检查返回的帧。

更多细节见[开发文档](doc/develop.md)。

### 报问题

- 用 [GitHub Issues](https://github.com/LinPr/grafana-openobserve-datasource/issues) 页面
- 提供详细复现步骤
- 附上相关日志和截图

### 提需求

- 先看有没有已有的 issue
- 清楚描述你的使用场景
- 说明期望的行为

### 贡献代码

有好想法欢迎直接联系，或者提 PR 加新功能、改进代码。

---

## 🩺 排障

| 现象                                                     | 原因与处理                                                                                                           |
| -------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| 数据源列表里找不到插件                                   | Grafana 默认不加载未签名插件。设置 `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource` 后重启。 |
| 插件列表里有 _Plugin is not signed_ 告警                 | 自己构建或从 release 压缩包安装时是正常的；从插件市场安装后就不再出现。                                              |
| Panel 空白但没有报错                                     | 通常是 stream 类型不匹配——把 `metrics` 流当成 `logs` 查了。后端会在 panel 的错误/提示区说明结果为空的原因。          |
| **Query mode** 里没有 `PromQL`                           | 它只对 `metrics` stream 类型开放，并且要求数据源设置里打开了 _Enable PromQL_。                                       |
| **Save & test** 报 `connection refused` / `no such host` | URL 必须是 Grafana **服务端**能访问的，不是你的浏览器。在 Docker 里，`localhost` 指的是容器自己。                    |
| **Save & test** 报 `401` / `403`                         | 检查 OpenObserve 组织、用户名/密码，或者确认 token 是原样发送的（`Basic <base64>` 和 `Bearer <jwt>` 都支持）。       |
| 时序图没有时间轴                                         | 时间列没被识别。显式设置 _Time field_，或者给它起别名（`AS ts`）并放进 `GROUP BY`。                                  |

---

## 🙏 致谢

- 感谢 [Grafana](https://github.com/grafana/grafana) 的维护者，本项目的 SQL 查询编辑器基于
  `@grafana/ui` 构建。
- 感谢 [OpenObserve](https://github.com/openobserve/openobserve) 的维护者在搜索 API 上的
  帮助和大量细致的技术解答。
- 感谢 [简道云](https://github.com/jiandaoyun) 团队对项目的支持，以及帮助改进产品的反馈。

## 📄 许可证

本项目使用 **Apache License 2.0** 许可——详见 [LICENSE](LICENSE)。
