# FindMyClassmate

> 校园学生档案检索台

FindMyClassmate 是一个面向校园场景的班级查询工具。React + TypeScript 前端构建后嵌入 Go 标准库服务，由后端统一托管页面、接口和年级名单，单二进制即可部署。

## 功能

- 支持高一、高二、高三名单查询：姓名、班级、年段自由组合（年段按数据目录实际存在的文件自动识别）
- 输入框支持空格 / 中文逗号 / 英文逗号 / 顿号分隔多条件
- 结果按完整、前缀、包含匹配排序，支持分页加载（全员果冻融合动效）
- 胶囊搜索框带彩色光束动画，搜索时思维球反馈
- 隐私设计：不写 localStorage，不把查询词写入 URL，API 只返回姓名 / 年级 / 班级

## 技术栈

| 层 | 技术 |
| --- | --- |
| 前端 | React 18 + TypeScript + Vite |
| 后端 | Go 标准库 HTTP 服务（单二进制） |
| 动效 | border-beam 1.3.0 / thinking-orbs 0.3.1 / liquid-gooey 0.2.1 |
| 字体 | Mona Sans / Instrument Sans / IBM Plex Mono |
| 测试 | Vitest（前端）、Go test（后端） |

## 快速开始

前置：Node.js 20+、Go 1.26+。

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

## 站点文案配置

页脚展示的内容（数据来源、运营团队、数据处理方）通过 `src/site.config.ts` 集中配置，无需改动业务代码：

```ts
const siteConfig = {
  /** 页脚：数据来源说明 */
  dataSource: "公示数据提取",
  /** 页脚：运营团队名称 */
  team: "信息社",
  /** 页脚：隐私说明中的数据处理方 */
  dataController: "信息社",
};
```

字段留空时对应区域不渲染（例如不需要在隐私说明中提及数据处理方，将其设为 `""` 即可）。

## 数据格式

名单数据为 JSON 文件，支持 `高一.json`、`高二.json`、`高三.json`（**真实名单不随仓库发布**，部署时自行放置，可按需要只放部分年段），结构如下：

```json
{
  "标题": "2025级高一编班名单",
  "名单": {
    "1班": [ { "姓名": "张三" } ]
  }
}
```

- 顶层仅 `标题` 与 `名单` 两个键；标题中的年段必须与文件名一致。
- 班级键格式为 `N班`（阿拉伯数字）；学生条目仅 `姓名` 一个字段（白名单）。
- 数据按年级、班级、姓名（去除空白、统一大写后）去重；API 不会读取或返回任何额外字段。

## 查询契约

- 中文逗号、英文逗号、顿号、连续空白均可分隔查询 token
- 姓名匹配键删除中英文空格、全角空格与制表符，统一大小写
- `高一` / `高二` / `高三` 为年段筛选（`高1` / `高2` / `高3` 为别名）；`18` 或 `18班` 为班级条件
- 结果按完整、前缀、包含匹配排序，同分按年段序列（高一 < 高二 < 高三）再按班级号升序
- API 响应固定为分页结构 `{ items, total, limit, offset, hasMore }`，首屏默认 10 条，单次最多 50 条

## 验证

前端：

```bash
npm run typecheck
npm test
npm run build
```

后端：

```bash
cd server
gofmt -l .          # 应无输出
go test ./...
go vet ./...
```

CI（GitHub Actions）在每次 push 时自动执行以上全部检查，详见 `.github/workflows/ci.yml`。

## 发布

打 tag 触发自动发布：

```bash
git tag v0.5.3
git push origin v0.5.3
```

`.github/workflows/release.yml` 会执行全量测试，然后：
1. 交叉编译 Linux amd64 / macOS arm64 / Windows amd64 三平台二进制（内嵌前端页面与 API 服务），连同示例数据与文档打包成 `findmyclassmate.tar.gz` 并创建 GitHub Release
2. 构建并推送 Docker 镜像到 Docker Hub（`wenxiloveyou/find-my-classmate`），标签为 `{version}`、`{major}.{minor}`，main 分支额外推送 `latest`

> Docker 推送需要仓库配置 `DOCKER_PAT` Secret（Docker Hub 访问令牌）。

## 目录结构

```
├── .github/workflows/   # CI 与发布流水线
├── data/                # 名单数据（高一.json / 高二.json / 高三.json，按需放置）
├── design/              # Logo 设计源文件
├── docs/                # 实施计划与架构文档
├── public/              # 静态资源（favicon / logo）
├── server/              # Go 服务（嵌入 server/web/ 构建产物）
├── src/                 # React 前端源码与测试
└── LICENSE              # MIT License
```

## 隐私与安全边界

- 页面不写入 localStorage，不把查询词或结果写入 URL
- API DTO 只返回 name、grade、class
- 服务端已内置：CSP 与安全响应头（防点击劫持 / 注入）、按 IP 令牌桶限流（每 IP 突发 60 个请求、稳态 1 请求/秒，429 返回 JSON + Retry-After 整数秒）、HTTP 读写超时（防慢速攻击）、访问日志脱敏且不记录查询参数
- 生产部署建议在边缘或网关补充：严格 CORS、HTTPS（HSTS）、反代层限流
- 部署形态边界：服务监听 0.0.0.0:3078（非标准端口）。若前置反代（Nginx/Caddy），必须处理：限流与脱敏日志基于 RemoteAddr，反代后所有客户端坍缩为反代 IP（限流退化为全局单桶、脱敏失效）——解决方案为反代层限流 + 可信来源白名单解析 XFF + 防火墙限制 3078 仅反代可达；若公网直连暴露，必须 TLS（当前为明文 HTTP，同网段可嗅探查询词，仅限内网部署）
- 名单数据含真实姓名（未成年人），发布前必须获得学校书面公示许可；**真实名单不进仓库、不进发布包**，仅限校内部署时自行放入 data/ 目录（数据格式见上文）

## 数据维护与备份
- **单实例边界**：限流桶与数据快照均为进程内状态，不支持多副本横向扩展；数据更新用原子替换（写临时文件后 mv），随后请求验证。

- 名单数据不进 git 仓库（.gitignore 排除），备份依赖本地文件系统 + 手动归档；恢复用重新放置文件 + 热重载秒级生效。
- 数据更新流程：源文件归档 → 规范化（字段白名单/班级键 N班/标题-文件名一致）→ 校验（`go test` + CI 数据契约）→ 人工复核（班级人数 10-60 区间，异常小班需回源核对）→ 独立提交 + CHANGELOG 条目。

## 贡献

1. Fork 本仓库并创建特性分支
2. 修改后运行全部验证命令（前端 typecheck / test / build，后端 gofmt / go test / go vet）
3. 提交信息使用中文或英文均可，保持简洁描述
4. 发起 Pull Request，CI 会自动运行全量检查

## License

[MIT](LICENSE) © FindMyClassmate Contributors