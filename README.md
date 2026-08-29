# FindMyClassmate

FindMyClassmate 是一个面向校园场景的班级查询工具。React + TypeScript 前端构建后嵌入 Go 标准库服务，由后端统一托管页面、接口和年级名单。

## 技术栈

- React 18 + TypeScript + Vite
- Go 标准库 HTTP 服务
- border-beam 1.3.0：搜索轨道的 monochrome line beam，请求中增强
- thinking-orbs 0.3.1：搜索中 searching，完成后 paused solving
- liquid-gooey 0.2.1：结果列表的 morph shape 液体轮廓与弹性进入
- Mona Sans：标题和大数字
- Instrument Sans：正文与中文混排
- IBM Plex Mono：字段、编号和状态标签
- Vitest：前端查询契约和搜索时序测试

## 交互

- 输入姓名、班级或年段后按 Enter，或点击开始搜索，立即发起查询。
- 搜索反馈至少持续 500ms，确保 searching 点状思维球有稳定展示时间。
- 输入法组合期间不会误提交；Escape 清空输入、结果和当前请求。
- 首次查询完成后页面平滑定位到查询结果；继续加载只追加结果，不改变当前位置。
- 触控目标和结果布局适配移动端，长姓名会自动换行。

## 启动

安装前端依赖：

    npm install

启动 Go 服务。服务从 data/高一.json 和 data/高二.json 读取名单，文件变化后会在下一次请求时自动热重载：

    go run ./server

浏览器打开 http://localhost:3078。Go 服务直接托管最新前端构建产物和 /api 接口。开发时可使用 npm run build 更新嵌入目录 server/web/。

## 验证

前端：

    npm run typecheck
    npm test
    npm run build

后端：

    cd server
    gofmt -w *.go
    go test ./...
    go vet ./...

## 查询契约

- 姓名支持中文或英文逗号、顿号、空格分隔，并按去空格后的键做包含匹配。
- 高一、高二是年段筛选；18 或 18班可作为班级条件。
- 服务统一返回分页响应 { items, total, limit, offset, hasMore }；默认每页 10 条，最多 50 条。
- 结果只返回姓名、年级和班级。

## 目录

- src/：React 页面、样式、查询逻辑、搜索时序和测试
- server/：Go 服务、JSON 归一化、查询测试
- docs/superpowers/plans/：实施计划
- DESIGN.md：视觉参考
- CLAUDE.md：项目长期记忆

生产部署仍需在边缘或网关配置 IP 限流、严格 CORS、脱敏日志和监控字段；这些配置不写死在本地原型中。
