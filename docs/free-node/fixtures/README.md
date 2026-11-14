# P0 测试夹具

这些夹具定义 P1/P2 必须实现的确定性测试，不保存完整代理链接、UUID、私钥、凭据、生产来源或用户数据。需要协议解析的实现测试应在各自代码库中通过安全的测试工厂构造临时输入，测试结束即销毁；不得将可导入节点样本提交到本仓库。

## 服务端来源与安全夹具

| ID | 输入概述 | 预期结果 |
| --- | --- | --- |
| `source-public-unauthenticated` | 公开、无需认证的 HTTPS 来源 | 可登记为启用来源 |
| `source-auth-required` | 要求登录、Cookie、验证码或令牌的来源 | 拒绝登记，审计不含认证材料 |
| `source-third-fetch-failure` | 同一来源连续三次超时/失败 | 第三次后暂停，已有有效输出不被覆盖 |
| `source-explicit-reenable` | 已暂停来源的显式启用操作 | 仅在全量校验通过后恢复；定时任务不恢复 |
| `target-loopback` | DNS 解析至回环地址 | 在连接前拒绝 |
| `target-private` | DNS 解析至私网、链路本地或保留地址 | 在连接前拒绝 |
| `target-rebind` | 首次解析为公网、连接前解析为非公网 | 连接前拒绝 |
| `output-failed-batch` | 新批次任一发布前置检查失败 | 保留最后有效订阅 |
| `retention-expiry` | 各类记录恰好到达保留期限 | 按 P0 保留期删除；不删当前有效输出 |

## 客户端确定性夹具清单

| ID | 初始状态与事件 | 预期结果 |
| --- | --- | --- |
| `fingerprint-replacement` | 相同连接相关字段、不同订阅 GUID/位置 | 关联同一历史状态 |
| `fingerprint-secret-change` | 连接认证材料变化 | 生成不同指纹，不继承旧状态 |
| `migration-previous-format` | 读取上一发布格式的本地状态 | 迁移后可读取且不丢失有效历史 |
| `single-failure` | 健康当前节点一次探测失败 | 不切换、不删除 |
| `single-latency-win` | 挑战者单次延迟较低 | 不抢占健康当前节点 |
| `repeated-advantage` | 挑战者重复测量且持续显著更优 | 仅达到阈值后切换 |
| `failure-isolation` | 达到连续失败阈值 | 隔离至 TTL；TTL 前不参与常规抽样 |
| `isolation-recovery-sample` | TTL 到期 | 仅作为有限恢复样本测试 |
| `subscription-replacement` | 新候选、历史重关联、过期清理中途失败 | 恢复或移除部分写入，保持一致状态 |

客户端夹具是 P2/P3 的测试清单，不表示这些功能在 P0 已实现。
