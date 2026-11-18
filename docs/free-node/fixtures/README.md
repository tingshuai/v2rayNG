# FN-2.0 确定性测试夹具清单

本清单定义 FN-2.0 验收场景。当前 Go 和 Android 测试覆盖情况列于下表；“未覆盖”表示没有直接断言该场景预期的自动化测试，不以相邻实现或手工线上检查替代。夹具不得保存生产来源、完整代理链接、UUID、私钥、凭据或用户数据；需要协议输入时由测试工厂临时生成并销毁。

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

| 状态 | 场景 ID | 现有测试证据 |
| --- | --- | --- |
| 已覆盖 | source-fetch-failure | `TestFailedSourceRunDoesNotReplacePriorCandidateInventory`、`TestThirdFailedRunPausesSourceAndExplicitEnableRestoresIt`、`TestBatchPublishesAcceptedCandidatesAfterScreeningDeadline`；来源失败保留旧候选关联，连续失败会暂停来源，屏蔽失败不会抹去旧发布 |
| 已覆盖 | source-missing-node、source-all-missing | `TestCandidateRetentionRequiresSuccessfulSourceInventoryAbsence`；仅一来源确认缺席时保留，所有关联来源确认且超过期限后删除 |
| 已覆盖 | source-different-cadence | `TestSourceScreeningCadencesAndInventoriesRemainIndependent`；快慢来源到期状态独立，快来源更新不覆盖慢来源的关联记录 |
| 已覆盖 | source-long-list-rotation、failed-probe-cooldown | `TestRotateCandidatesAdvancesThroughLongSourceAndWraps`、`TestCandidateProbeStateAppliesCooldownAndRepeatedFailureIsolation` |
| 已覆盖 | duplicate-across-sources | `TestDuplicateCandidateAcrossSourcesIsFullyProbedOnlyOnce`、`TestDuplicateCandidateAfterSuccessfulProbeIsSkippedForAnotherSource`；并发及顺序输入均只完整探测一次 |
| 已覆盖 | unsafe-revocation | `TestSecurityRevocationPersistsUntilExplicitRestore`、`TestCandidateManifestCarriesBoundedCandidatesDigestAndRevocation`、Android `FreeNodeHistoryMigratorTest` 的撤销用例 |
| 已覆盖 | weighted-distribution | `TestWeightedSelectionReducesConcentrationFromOneSource`、`TestWeightedSelectionGuaranteesThreeSourcesWhenAvailable`、`TestDistributionEnforcesDailyCandidateCapacityEvenBelowFeedLimit` |
| 已覆盖 | publish-empty | `TestPublishAtomicallyKeepsPreviousOutputAsRollback`、`TestPublicationRollbackRemovesFirstOutputWhenAuditCannotBeRecorded` |
| 已覆盖 | client-merge、client-failure、incumbent-stable | Android `FreeNodeHistoryMigratorTest`、`FreeNodeScoringTest` 覆盖近期有效节点保留、重复失败隔离及挑战者滞回 |
| 已覆盖 | feedback-disabled、feedback-single-failure、feedback-privacy | `FreeNodeFeedbackReportTest`、`TestFeedbackEndpointValidatesAndPassesOnlyCoarseAggregateFields`、`TestFeedbackIsStoredAsBoundedDailyAggregateAndExpires`、`TestCandidateDistributionWeightRewardsRecentSuccessAndLimitsPressure` |
| 已覆盖 | data-expiry | `TestPruneExpiredRemovesOnlyExpiredRecords`、`TestFeedbackIsStoredAsBoundedDailyAggregateAndExpires`、`TestRemoveExpiredBackupRetainsOnlyRecentRollbackOutput` |
| 未覆盖 | network/battery constraints、worker cancellation、VPN/proxy-only/root compatibility | 当前夹具清单没有覆盖 WorkManager 约束、取消或三种服务模式的统一运行时回归场景 |

“未覆盖”项是后续自动化工作，不影响已覆盖用例的结论。涉及 Android WorkManager/服务生命周期的场景需要设备或模拟器，不能由纯 JVM 夹具替代。

目前补齐了来源周期独立、失败后旧候选关联保留、跨来源重复候选成功去重与失败后重试、以及 Android 最近成功候选数量和所选指纹迁移的确定性检查。仍待增加：Android WorkManager 网络/电量约束与取消检查；VPN、仅代理、root 受影响路径的设备回归。并发覆盖现有全局探测并发/预算和新增的候选去重用例；事务恢复覆盖发布回滚，但不代表所有数据库多记录事务失败路径均已覆盖。
