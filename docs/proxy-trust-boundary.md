# 反代信任边界（Proxy Trust Boundary）

本文档记录 FindMyClassmate 在反向代理（Cloudflare + Nginx）部署形态下的客户端 IP 信任边界与完整性防线。它是 `FMC_TRUST_PROXY_HEADERS` 功能（应用层回环门 + 部署层改写）的可审计描述：**任何部署此功能的实例必须同时满足本文档的应用层与部署层两层约束**。

## 1. 威胁模型

反代场景下，到达应用的请求 `RemoteAddr` 恒为回环/网桥地址（本部署为 Nginx 从 `127.0.0.1` 回源），应用无法从 TCP 层区分来源。若应用无条件信任转发头（`CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For`），直连源站 443（绕过 Cloudflare）的攻击者可以携带任意伪造值，使限流键随伪造 IP 变换（换头即换桶），限流完全失效，进而全量枚举名单。

实测（2026-11-17，首尔）：修复前直连 `443` + `CF-Connecting-IP: 203.0.113.99`，应用访问日志出现 `203.0.*.*`（伪造值被采信）。

## 2. 应用层边界（代码内，`server/ip.go`）

- `FMC_TRUST_PROXY_HEADERS` 默认关闭：不信任任何转发头，限流键取 `RemoteAddr`。
- 开启时仅当 `RemoteAddr` 是回环（`127.0.0.0/8`、`::1`，即本地反代唯一入口）才读取转发头：`CF-Connecting-IP → X-Real-IP → X-Forwarded-For 首值`，每个候选经 `net.ParseIP` 校验，非法即回退；全无效回退 `RemoteAddr`。
- 非回环来源一律忽略转发头回退 `RemoteAddr`（直连来源无法让 RemoteAddr 成为回环）。
- 实现：`clientIPFromRequest`（限流键与访问日志脱敏共用）。

应用层回环门**不能单独成立**：它只证明连接来自本机反代，不证明转发头是反代写的。转发头可信度由部署层保证。

## 3. 部署层三层防线（首尔现状，2026-11-17 落地）

| 层 | 配置 | 位置 |
|---|---|---|
| ① ufw | 443/tcp 入站仅允许 Cloudflare IP 段（20 条 `cloudflare-origin` 规则），无 `ALLOW Anywhere` | `ufw status` |
| ② nginx stream | 443 `include /etc/nginx/stream-cf-allow.conf`（Cloudflare 段 `allow` + `deny all`）；非 Cloudflare 来源回源 `status=403` | `/etc/nginx/nginx.conf` stream 块 |
| ③ find 站点 | `include /etc/nginx/http-find-realip.conf`（`set_real_ip_from 127.0.0.1`、`::1`）+ `real_ip_header CF-Connecting-IP` + `proxy_set_header CF-Connecting-IP $remote_addr`（改写后显式覆盖透传头） | `/etc/nginx/sites-available/find.wenxi.dev` |

真实性核对：`nginx -t` 通过后 `systemctl reload nginx`；配置备份 `/root/nginx-cf-fix-20261117_122957/`。

## 4. 验证命令

```bash
# 正向：经 CF 访问，应用日志应显示真实客户端 IP 脱敏（如 101.33.*.*），而非 127.0.*.*
curl -s https://find.wenxi.dev/api/schools
docker logs findmyclassmate | grep "GET /api/schools" | tail -2

# 反向：直连源站 443（绕过 CF），应连接失败（stream 403 / 超时），应用无新日志
curl -sk --resolve find.wenxi.dev:443:127.0.0.1 -H "CF-Connecting-IP: 203.0.113.99" \
  https://find.wenxi.dev/api/health   # 期望失败（HTTP 000）
```

## 5. Cloudflare IP 段变更运维

Cloudflare 会调整回源 IP 段，需定期同步（或接入自动化 cron）：

```bash
curl -s https://www.cloudflare.com/ips-v4 -o /tmp/cf-v4.txt
curl -s https://www.cloudflare.com/ips-v6 -o /tmp/cf-v6.txt
# 重生成 /etc/nginx/stream-cf-allow.conf（每段一行 allow <段>; + deny all;）
nginx -t && systemctl reload nginx
# ufw 同步：删除旧段规则，添加新段规则（ufw allow from <段> to any port 443 proto tcp comment cloudflare-origin）
```

## 6. 应用层与部署层的关系

- 部署层防线（本文档第 3 节）负责「只让 Cloudflare 真实回源到达应用 + 改写/覆盖转发头为可信值」。
- 应用层回环门（第 2 节）是第二道校验：即使部署层被误配置（如白名单缺失），非回环来源的伪造头仍会被应用拒绝。
- 两层叠加才是完整防线；只部署其一不构成「双层防线」。