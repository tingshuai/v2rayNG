# P4 端到端试运行记录

## 目标与判定

P4 验证已发布候选订阅、客户端的本地评分边界和部署侧数据控制能一起工作；它不是新的订阅格式或第二套选路实现。通过条件仍以 `development-roadmap.md` 的 P4 完成标准为准。此记录把每次可复现的试运行证据和未满足的前置条件分开，避免把公开端点可达误报为完整验收。

## 2026-11-11：启动基线

以下检查在不输出候选完整配置、凭据、UUID、私钥或客户端网络数据的条件下完成：

| 检查 | 结果 | 证据与范围 |
| --- | --- | --- |
| 采集器确定性回归 | 通过 | `cd collector && go test ./...`。|
| Android 免费节点确定性回归 | 通过 | `cd V2rayNG && gradlew.bat :app:testPlaystoreDebugUnitTest --tests "com.v2ray.ang.handler.FreeNode*" --console=plain`；28 个 Gradle task 完成。|
| Android 完整 JVM 与编译检查 | 通过 | `cd V2rayNG && gradlew.bat :app:testPlaystoreDebugUnitTest :app:compilePlaystoreDebugKotlin --console=plain`；完整 JVM 测试和 Play Store Debug Kotlin 编译均完成。|
| 公开发布链路只读检查 | 通过 | 配置的健康检查端点返回 204，订阅端点返回 200。响应可 Base64 解码为 47 条标准订阅记录，协议仅为 `ss`、`trojan`、`vless`；检查未发现 RFC1918 或 loopback IPv4 目标。此检查不记录或保存任何记录正文。|
| 真机 Wi-Fi 启用与后台任务 | 通过 | 在同一真机的已验证 Wi-Fi 上，经风险提示确认后启用免费节点。专用候选订阅与本地策略组被导入，受限 `free-node-probe` WorkManager 工作被调度并以 `SUCCESS` 完成。关闭功能后，配置页仅保留原有配置，探测工作已取消。|
| 真机关闭态回归（VPN、仅代理、root） | 通过 | 在一台已获 Magisk 授权、支持标准 `su -c` 的 Android 10 真机上，先关闭免费节点并确认专用候选与策略组从配置页消失、`free-node-probe` 不存在。VPN 与仅代理模式均可成功启动、停止且未创建该任务；root 模式成功启动和停止 `CoreRootService`，运行期出现后又清理 root 隧道进程。结束后恢复 VPN 模式与关闭 root 模式。|

## 尚待执行的受控试运行

以下检查需要隔离测试环境或部署维护者权限，因而没有用公开端点替代：

1. 在独立测试来源上依次验证更新、替换、来源暂停、候选失效和恢复；每次只保存来源级聚合结果与订阅内容散列。
2. **Not run（用户确认延期）**：在可用的计费网络上验证最多 8 个候选的测量上限、低电量/无网络延后、隔离 TTL 和挑战者两次优势滞回；确认不会改写用户当前选中的服务器。当前真机未注册蜂窝数据服务，用户于 2026-11-11 确认本轮不测试此项。
3. 在部署端审计 SQLite、运行日志、临时探测目录和回退输出：确认不会保存完整代理 URL、认证材料、UUID、私钥、客户端 IP 或测试载荷；同时确认过期临时配置和回退副本按运行手册清理。

完成这些项目后，按 `collector-runbook.md` 的事故与保留规则更新本记录，并把结果回填到路线图。任何失败应保留非秘密的时间、组件、来源 ID（如适用）和聚合计数，而不是候选内容。
