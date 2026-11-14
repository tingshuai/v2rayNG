# FN-2.0 确定性测试夹具清单

本清单定义目标行为，尚未实现。夹具不得保存生产来源、完整代理链接、UUID、私钥、凭据或用户数据；需要协议输入时由测试工厂临时生成并销毁。

| ID | 场景 | 预期 |
| --- | --- | --- |
| source-different-cadence | 两来源更新周期不同 | 各自到期获取，候选合并，慢来源不会被快来源更新覆盖 |
| source-fetch-failure | 来源拉取失败 | 不将其旧节点标记为消失 |
| source-missing-node | 节点从一个来源缺席、仍在另一来源出现 | 保留节点及来源关联 |
| source-all-missing | 所有关联来源成功更新且节点持续缺席 | 达到保留期后过期 |
| duplicate-across-sources | 相同连接字段出现在多个来源 | 一个指纹、一次完整探测 |
| source-long-list-rotation | 来源长期返回稳定长列表 | 每轮位置推进，尾部节点最终获得测试机会 |
| failed-probe-cooldown | 节点连续失败 | 隔离期间不重测，到期有限复测 |
| unsafe-revocation | 配置或地址安全违规 | 立即停发并产生撤销信号 |
| weighted-distribution | 质量与分配压力不同 | 好节点有更多机会，集中度有上限，新节点仍获份额 |
| publish-empty | 新发布为空或中途失败 | 保留最后有效清单与回退副本 |
| client-merge | 新清单缺少本机近期有效节点 | 保留旧节点并继承历史 |
| client-failure | 本机网络正常但某节点重复失败 | 退出可选集合，暂留失败指纹 |
| incumbent-stable | 当前节点健康、挑战者单次更快 | 不切换 |
| feedback-disabled | 用户关闭反馈 | 不发送报告，清单与选路继续工作 |
| feedback-single-failure | 一份匿名失败报告 | 最多降权/安排复测，不全局删除 |
| feedback-privacy | 反馈与日志审计 | 无配置、凭据、UUID、设备 ID、IP 或测试载荷 |
| data-expiry | 各类数据到达保留期限 | 删除过期数据且不删当前有效发布 |

实现测试须覆盖日期边界、并发、事务失败恢复、网络/电量约束和 VPN、仅代理、root 受影响路径。
