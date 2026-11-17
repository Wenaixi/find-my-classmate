# FindMyClassmate

> 校园学生档案检索台

[![CI](https://github.com/Wenaixi/find-my-classmate/actions/workflows/ci.yml/badge.svg)](https://github.com/Wenaixi/find-my-classmate/actions/workflows/ci.yml)
[![Release](https://github.com/Wenaixi/find-my-classmate/actions/workflows/release.yml/badge.svg)](https://github.com/Wenaixi/find-my-classmate/actions/workflows/release.yml)
![Go](https://img.shields.io/badge/Go-1.27.4-00ADD8?logo=go&logoColor=white)
![Node](https://img.shields.io/badge/Node-26_LTS-339933?logo=nodedotjs&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![Vite](https://img.shields.io/badge/Vite-8-646CFF?logo=vite&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-7-3178C6?logo=typescript&logoColor=white)
![License](https://img.shields.io/github/license/Wenaixi/find-my-classmate)

FindMyClassmate 是一个面向校园场景的班级查询工具。React + TypeScript 前端构建后嵌入 Go 标准库服务，由后端统一托管页面、接口和年级名单，单二进制即可部署。

## 功能

- 支持高一、高二、高三名单查询：姓名、班级、年段自由组合（年段按数据目录实际存在的文件自动识别）
- 多学校：data/ 目录下每校一个 JSON，学校筛选胶囊下拉，API 返回学校字段
- 输入框支持空格 / 中文逗号 / 英文逗号 / 顿号分隔多条件
- 结果按完整、前缀、包含匹配排序，支持分页加载（全员果冻融合动效）
- 胶囊搜索框带彩色光束动画，搜索时思维球反馈
- 隐私设计：不写 localStorage，不把查询词写入 URL，API 只返回姓名 / 学校 / 年级 / 班级

## 技术栈

| 层 | 技术 |
| --- | --- |
| 前端 | React 19 + TypeScript 7 + Vite 8 |
| 后端 | Go 1.27 标准库 HTTP 服务（单二进制） |
| 动效 | border-beam 1.4.1 / thinking-orbs 0.3.2 / liquid-gooey 0.2.2 |
| 字体 | Mona Sans / Instrument Sans / IBM Plex Mono |
| 测试 | Vitest 5（前端）、Go test（后端） |

## 快速开始

前置：Node.js 26（LTS，与 CI 一致）、Go 1.27.4（go.mod 声明；本机低版本工具链会经 GOTOOLCHAIN 自动获取）。

```bash
# 1. 安装前端依赖
npm install

# 2. 构建前端（产物输出到 server/web/）
npm run build

# 3. 启动 Go 服务（默认端口 3078）
go run ./server

# 4. 浏览器打开 http://localhost:3078
```

数据目录默认 `./data`，可通过环境变量 `FMC_DATA_DIR` 指向其他目录。名单文件变更后下一次请求自动热重载；日志写入 `FMC_LOG_DIR`（默认 `<数据目录>/log`），容器场景建议显式指定可写目录。

## 部署安全边界

本服务是**无认证的公开查询工具**，按「校园内网/受控网络」的部署前提设计。以下边界请部署前确认：

- **名单可全量枚举（隐私边界）**：`/api/search` 无鉴权，单字查询即可返回全校匹配，全部学生（姓名/学校/年级/班级，PII）约数百次请求即可完整拉取。内网部署可接受；**服务一旦暴露到公网，等于公开全部名单**。公网部署必须置于访问控制之后：校园 VPN、IP 白名单、反向代理鉴权（如 Cloudflare Access），或把端口绑定到本机回环（`docker-compose.yml` 已附 `127.0.0.1` 绑定示例）。
- **限流按 IP 分桶（双栈注意）**：限流键取客户端 IP（容量 60、每秒回补 1）。服务同时监听 IPv4 与 IPv6 时，同一主机的 `127.0.0.1` 与 `::1` 是两个独立桶，双栈攻击者可获得双倍配额。缓解：部署层只暴露单一协议栈；这是按 IP 限流的结构性特性。
- **反代部署下的真实客户端 IP**：`FMC_TRUST_PROXY_HEADERS=true` 时，应用按 `CF-Connecting-IP → X-Real-IP → X-Forwarded-For 首值` 取真实客户端 IP 作限流键（仅当连接来自回环地址即本地反代唯一入口才信任转发头；非回环来源一律忽略回退 RemoteAddr）。该开关默认关闭；开启后**部署层必须用可信来源白名单改写并覆盖转发头**（如 Nginx real_ip + 仅允许 Cloudflare 回源），否则直连攻击者伪造转发头即可绕过限流。完整边界见 `docs/proxy-trust-boundary.md`。
- **版本号公开**：`/api/health` 返回 `version`（ldflags 注入的 git tag，本地为 `dev`），用于运维溯源。如需对公网隐藏，在反代层剥离该字段即可。
- **日志含运维细节**：错误根因日志（名单文件名、班级名）只写服务端日志（文件权限 0600），API 响应始终只返回错误码，不含内部细节。

## 站点文案配置

页脚展示的内容（数据来源、运营团队）通过 `src/site.config.ts` 集中配置，无需改动业务代码：

```ts
const siteConfig = {
  /** 页脚：数据来源说明 */
  dataSource: "福清一中公示数据提取",
  /** 页脚：运营团队名称 */
  team: "福清一中信息社",
};
```

## 数据格式

名单数据为 JSON 文件（**真实名单不随仓库发布**，部署时自行放置）。支持多学校：data/ 目录下每校一个 `*.json`，文件名不参与语义（可任意命名），学校身份取自文件内的 `校名` 字段。推荐使用 `build_hierarchy_json.py --no-credential` 导出的纯净名册（无账号密码、无 id 字段）结构如下：

```json
{
  "区县": "福清市",
  "学校": [{
    "校名": "福清一中",
    "年级": [{ "年级名": "高一", "班级": [{ "班名": "一班", "学生": [{ "姓名": "张三" }] }] }]
  }]
}
```

- 顶层仅 `区县` 与 `学校` 两个键；`学校` 为数组，每项含 `校名` 与 `年级`（数组）。
- 年级项含 `年级名` 与 `班级`（数组）；班级项含 `班名` 与 `学生`（数组）；学生仅 `姓名` 一个字段。
- 文件内可含多个学校、多个年级；跨文件校名重复会被拒绝加载。
- API 返回 `name/school/grade/class` 四个字段；学校下拉（`/api/schools`）只列出包含高中年级（高一~高三）的学校。
