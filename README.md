<div align="center">

# ⚡ WRT AI

**运行在 OpenWRT 路由器上的 AI 助手 —— 和路由器对话，让它替你干活**

An AI assistant that runs on your OpenWRT router: chat in a browser, and the AI reads system status, digs through logs, runs network diagnostics and modifies UCI configuration — through a graded approval + auto-backup safety system.

</div>

---

## 功能特性

- **🤖 AI 操作路由器**：内置 10 个工具 —— 系统信息、网络/无线状态、在线设备、系统日志、ping/traceroute/nslookup 诊断、UCI 读写、服务管理、配置重载。AI 自主决定调用链，查状态 → 分析 → 提方案 → 执行。
- **🛡️ 分级护栏**：
  | 级别 | 示例 | 策略 |
  |------|------|------|
  | 只读 | 查状态、看日志、诊断 | 自动执行 |
  | 写操作 | 改 system 等配置、服务启停 | 默认需在界面点击确认（可关） |
  | 危险操作 | network / wireless / firewall / dhcp | **永远**需要确认 + 执行前自动备份，不可关闭 |
- **🔄 挂起/恢复的审批模型**：AI 提出危险修改时任务挂起，可以关掉浏览器改天再来批准；服务重启后等待中的审批依然有效；支持一键回滚备份。
- **🔌 自定义模型**：任意 OpenAI 兼容上游（OpenAI / DeepSeek / 通义 / Kimi / Ollama / 中转站），支持多个 Provider、连通测试、拉取模型列表。
- **💬 流式聊天界面**：SSE 流式输出、断线自动重连（换 Wi-Fi 不丢会话）、工具执行过程可视化、Markdown/表格渲染。
- **🧩 极薄 LuCI 集成**：LuCI 只提供「启用开关 + 端口」两项；模型、安全、备份全部在独立 Web 界面管理。
- **📦 单二进制**：Go 静态编译，前端构建产物直接嵌入，全架构（amd64 / arm64 / armv7 / armv6 / mips / mipsle）约 10~13MB，无外部依赖。

## 架构

```
浏览器 ──HTTP/SSE──▶ wrtai (Go, procd 托管, /etc/config/wrtai)
                      ├─ server   鉴权 / REST / SSE
                      ├─ agent    run 状态机 + function-calling 循环
                      ├─ safety   分级审批 + uci export 备份 + 回滚
                      ├─ tools    uci/ubus/logread/iwinfo/诊断 (真实|mock 同接口)
                      └─ llm      OpenAI 兼容客户端（流式 + 工具调用）
LuCI (luci-app-wrtai) ── 仅写 enabled + port ──▶ 重启服务
```

## 快速开始

### 方式一：apk 包（OpenWrt 25.12+，推荐）

从 [Releases](../../releases) 下载对应架构的 `.apk`，在路由器上：

```sh
apk add --allow-untrusted ./wrtai-aarch64_generic-0.1.1.apk   # 无自建签名，需 allow-untrusted
/etc/init.d/wrtai enable --now
```

架构对照：`aarch64_generic`（绝大多数新路由）、`mipsel_24kc`（MT7621 等）、`mips_24kc`、`x86_64`（软路由）。armv6/armv7 目标包名过于细分，请用方式三。

### 方式二：安装脚本（任意版本，通用）

下载对应架构的 `wrtai-linux-*.tar.gz` 解压到 `dist/linux-<arch>/wrtai`，然后：

```sh
./scripts/install.sh root@192.168.1.1          # 自动识别架构
./scripts/install.sh root@192.168.1.1 arm64    # 手动指定
```

脚本会安装二进制、注册 procd 服务、写入默认 UCI 配置（**不会覆盖已有配置**），并把 `/etc/wrtai/` 加入 sysupgrade 保留列表。

### 方式三：手动安装

```sh
scp dist/linux-arm64/wrtai root@192.168.1.1:/usr/bin/wrtai
scp openwrt/wrtai/files/wrtai.init root@192.168.1.1:/etc/init.d/wrtai
scp openwrt/wrtai/files/wrtai.uci root@192.168.1.1:/etc/config/wrtai   # 仅首次
ssh root@192.168.1.1 '/etc/init.d/wrtai enable && /etc/init.d/wrtai start'
```

### 方式四：buildroot 自行打包（opkg 24.10 及更早）

```sh
cp -r openwrt/wrtai <buildroot>/package/wrtai
cp -r openwrt/luci-app-wrtai <buildroot>/package/luci-app-wrtai
make package/wrtai/compile V=s
make package/luci-app-wrtai/compile V=s
# 25.12+ 的 buildroot 会产出 .apk，24.10 及更早产出 .ipk，同一份包描述通吃
```

### 首次使用

1. 浏览器访问 `http://<路由器IP>:8765`，设置管理密码
2. 进入 **设置 → 模型服务**，填入任意 OpenAI 兼容上游（如 `https://api.deepseek.com/v1` + API Key），点「测试」验证并选择模型
3. 回到聊天页，问一句「查看路由器状态」

> **安全提示**：服务默认只监听 LAN（OpenWRT 防火墙默认拦截 WAN 入站）。请不要手动在防火墙放通 WAN 侧端口。

## 开发

```sh
make dev        # 启动开发模式：mock 路由器 + 内置假模型，无需真实路由器和 API Key
                # → http://127.0.0.1:8765  (完整走通聊天/工具/审批/备份流程)
make test       # Go 测试（含审批挂起恢复、拒绝流程、重启恢复等集成测试）
make build      # 生产构建（前端 + 后端单二进制）
make cross      # 全架构交叉编译 → dist/
```

- 前端独立开发：`cd web && npm run dev`（Vite 代理 `/api` 到 8765）
- 开发模式自动注入「内置模拟模型」，聊天说「把防火墙 WAN 入站改成 ACCEPT」即可演示危险操作审批

### 真机验证清单

- [ ] `install.sh` 安装后服务自动启动，`logread | grep wrtai` 无报错
- [ ] LuCI → 服务 → WRT AI：关闭/改端口后保存并应用，服务跟随重启
- [ ] 聊天查系统状态 / 日志 / 在线设备（真实 uci/ubus 数据）
- [ ] 触发一次危险操作（如改无线 SSID）→ 确认卡片 → 批准 → 设置→备份 出现快照 → 一键回滚生效
- [ ] 审批挂起后杀掉 wrtai 进程重启 → 审批卡片依然可批准（重启恢复）
- [ ] 手机切 Wi-Fi/蜂窝网络后聊天流自动恢复

## 目录结构

```
├── cmd/wrtai/           # 入口
├── internal/            # config / llm / agent / tools / safety / runstate / store / server
├── web/                 # React + Vite + Arco Design 前端（构建产物经 go:embed 嵌入）
├── openwrt/wrtai/       # buildroot 包：procd init + UCI 默认配置
├── openwrt/luci-app-wrtai/  # LuCI 薄页（启用 + 端口）
├── scripts/install.sh   # SSH 一键安装
└── .github/workflows/   # CI + release（全架构产物）
```

## Roadmap

- [x] OpenWrt 25.12+ apk 包（Release 直接下载）
- [ ] apk 签名密钥与自建 feed 文档（免 --allow-untrusted）
- [ ] 危险操作 90 秒无确认自动回滚（防锁死保险）
- [ ] 用量统计与多会话管理增强
- [ ] 无线站点扫描工具（AI 选信道）
- [ ] 多语言界面（i18n）

## License

[MIT](LICENSE)
