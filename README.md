# FindMyClassmate

> 福清一中信息社 · 学生档案检索台

FindMyClassmate 是一个面向校园场景的班级查询工具。React + TypeScript 前端构建后嵌入 Go 标准库服务，由后端统一托管页面、接口和年级名单，单二进制即可部署。

## 功能

- 支持福清一中高一、高二名单查询：姓名、班级、年段自由组合
- 输入框支持空格 / 中文逗号 / 英文逗号 / 顿号分隔多条件
- 结果按完整、前缀、包含匹配排序，支持分页加载
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

Go 服务默认从 `data/高一.json` 和 `data/高二.json` 读取名单，文件变更后下一次请求自动热重载；日志写入 `data/log/server.log`。可用 `FMC_DATA_DIR` 环境变量指向其他数据目录。

## 数据格式

名单数据为两个 JSON 文件（data/高一.json、data/高二.json），结构如下：

```json
{
  "标题": "福清一中2025级高一编班名单",
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
- `高一`、`高二` 为年段筛选；`18` 或 `18班` 为班级条件
- 结果按完整、前缀、包含匹配与自然班级顺序排列
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
git tag v0.5.0
git push origin v0.1.0
```

`.github/workflows/release.yml` 会执行全量测试，然后：
1. 交叉编译 Linux amd64 / macOS arm64 / Windows amd64 三平台二进制（内嵌前端页面与 API 服务），连同示例数据与文档打包成 `findmyclassmate.tar.gz` 并创建 GitHub Release
2. 构建并推送 Docker 镜像到 Docker Hub（`wenxiloveyou/find-my-classmate`），标签为 `{version}`、`{major}.{minor}`，main 分支额外推送 `latest`

> Docker 推送需要仓库配置 `DOCKER_PAT` Secret（Docker Hub 访问令牌）。

## 目录结构

```
├── .github/workflows/   # CI 与发布流水线
├── data/                # 名单数据（高一.json / 高二.json）
├── design/              # Logo 设计源文件
├── docs/                # 实施计划文档
├── public/              # 静态资源（favicon / logo）
├── server/              # Go 服务（嵌入 server/web/ 构建产物）
├── src/                 # React 前端源码与测试
├── CLAUDE.md            # 项目长期记忆
├── DESIGN.md            # 视觉设计参考
└── LICENSE              # MIT License
```

## 隐私与安全边界

- 页面不写入 localStorage，不把查询词或结果写入 URL
- API DTO 只返回 name、grade、class
- 服务端已内置：CSP 与安全响应头（防点击劫持 / 注入）、按 IP 令牌桶限流（每 IP 突发 60 个请求、稳态 1 请求/秒，429 返回 JSON + Retry-After 整数秒）、HTTP 读写超时（防慢速攻击）、访问日志脱敏且不记录查询参数
- 生产部署建议在边缘或网关补充：严格 CORS、HTTPS（HSTS）、反代层限流
- 名单数据含真实姓名（未成年人），发布前必须获得学校书面公示许可；公开发布包中的 data-sample 为假名示例数据，真实名单仅限校内部署。数据处理者：福清一中信息社（删除/更正请联系学校教务处）。

## 贡献

1. Fork 本仓库并创建特性分支
2. 修改后运行全部验证命令（前端 typecheck / test / build，后端 gofmt / go test / go vet）
3. 提交信息使用中文或英文均可，保持简洁描述
4. 发起 Pull Request，CI 会自动运行全量检查

## License

[MIT](LICENSE) © FindMyClassmate Contributors（福清一中信息社）