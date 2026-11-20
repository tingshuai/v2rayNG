package com.v2ray.ang.handler

import android.content.Context
import android.util.Base64
import com.v2ray.ang.AppConfig
import com.v2ray.ang.R
import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.dto.entities.SubscriptionItem
import com.v2ray.ang.enums.BalancerStrategyType
import com.v2ray.ang.enums.EConfigType
import com.v2ray.ang.util.JsonUtil
import java.security.SecureRandom

/** Owns the opt-in free-node subscription and its locally created policy group. */
object FreeNodeManager {
    fun isEnabled(): Boolean = MmkvManager.decodeSettingsBool(AppConfig.PREF_FREE_NODE_ENABLED, false)

    fun isFeedbackEnabled(): Boolean = MmkvManager.decodeSettingsBool(AppConfig.PREF_FREE_NODE_FEEDBACK_ENABLED, true)

    fun setFeedbackEnabled(enabled: Boolean) {
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_FEEDBACK_ENABLED, enabled)
    }

    fun isDedicatedSubscription(subscriptionId: String): Boolean =
        subscriptionId == AppConfig.FREE_NODE_SUBSCRIPTION_ID

    fun enable(context: Context) {
        val selectedBefore = MmkvManager.getSelectServer()
        val existingSubscription = MmkvManager.decodeSubscription(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
        val subscription = existingSubscription ?: SubscriptionItem(
            remarks = context.getString(R.string.free_node_subscription_name),
            url = AppConfig.FREE_NODE_SUBSCRIPTION_URL,
            autoUpdate = true,
        )
        subscription.url = AppConfig.FREE_NODE_SUBSCRIPTION_URL
        subscription.autoUpdate = true
        MmkvManager.encodeSubscription(AppConfig.FREE_NODE_SUBSCRIPTION_ID, subscription)
        try {
            ensurePolicyGroup(context)
            restoreSelection(selectedBefore)
            ensureFingerprintMigration()
            MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_RISK_NOTICE_VERSION, AppConfig.FREE_NODE_RISK_NOTICE_VERSION)
            MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_ENABLED, true)
            FreeNodeProbeScheduler.schedule(context)
        } catch (error: RuntimeException) {
            disable()
            throw error
        }
    }

    fun disable() {
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_ENABLED, false)
        MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_POLICY_GROUP_ID)
            ?.let(MmkvManager::removeServer)
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_POLICY_GROUP_ID, "")
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_FINGERPRINT_KEY, "")
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_STATE_VERSION, 0)
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_HISTORY, "")
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_RISK_NOTICE_VERSION, 0)
        SubscriptionUpdater.cancelOne(subId = AppConfig.FREE_NODE_SUBSCRIPTION_ID)
        FreeNodeProbeScheduler.cancel()
        MmkvManager.removeSubscription(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
    }

    fun fingerprint(profile: ProfileItem): String {
        return FreeNodeFingerprint.create(profile, fingerprintKey())
    }

    fun migrateHistory(profiles: Collection<ProfileItem>, candidateIds: Map<String, String> = emptyMap()) {
        val previous = MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_HISTORY)
            ?.let { JsonUtil.fromJsonSafe(it, FreeNodeHistoryState::class.java) }
            ?: FreeNodeHistoryState()
        val migrated = FreeNodeHistoryMigrator.migrate(previous, profiles, candidateIds, ::fingerprint)
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_HISTORY, JsonUtil.toJson(migrated))
        if (isEnabled()) FreeNodeProbeScheduler.schedule()
    }

    fun retainRecentlyValidatedProfiles(incoming: Collection<ProfileItem>): List<ProfileItem> {
        val state = loadHistory()
        val existing = MmkvManager.decodeServerList(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
            .mapNotNull(MmkvManager::decodeServerConfig)
        return FreeNodeHistoryMigrator.recentlyValidatedProfiles(state, existing, incoming, ::fingerprint, System.currentTimeMillis())
    }

    fun selectableFingerprints(now: Long = System.currentTimeMillis()): Set<String> {
        val state = loadHistory()
        return state.entries.filterValues {
            it.remoteCandidateId !in state.revokedCandidateIds && FreeNodeHistoryMigrator.isSelectable(it, now)
        }.keys
    }

    fun applyFeedRevocations(candidateIds: Set<String>) {
        val state = loadHistory()
        val revokedFingerprints = FreeNodeHistoryMigrator.revokedFingerprints(state, candidateIds)
        val revokedProfiles = MmkvManager.decodeServerList(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
            .filter { guid ->
                MmkvManager.decodeServerConfig(guid)?.let { fingerprint(it) in revokedFingerprints } == true
            }
        MmkvManager.removeServers(revokedProfiles, AppConfig.FREE_NODE_SUBSCRIPTION_ID)
        if (state.revokedCandidateIds != candidateIds || revokedFingerprints.isNotEmpty()) {
            saveHistory(state.copy(
                revokedCandidateIds = candidateIds,
                entries = state.entries - revokedFingerprints,
                selectedFingerprint = state.selectedFingerprint?.takeUnless(revokedFingerprints::contains),
            ))
        }
    }

    fun probePlan(now: Long = System.currentTimeMillis()): Map<String, String> {
        val profiles = MmkvManager.decodeServerList(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
            .mapNotNull { guid -> MmkvManager.decodeServerConfig(guid)?.let { profile -> fingerprint(profile) to guid } }
            .toMap()
        val state = loadHistory()
        return FreeNodeScoring.plan(state, now).mapNotNull { fingerprint ->
            profiles[fingerprint]?.let { guid -> fingerprint to guid }
        }.toMap()
    }

    fun recordProbe(fingerprint: String, delayMillis: Long, now: Long = System.currentTimeMillis()): FreeNodeFeedbackReport? {
        val previous = loadHistory()
        val history = previous.entries[fingerprint] ?: return null
        val report = if (isFeedbackEnabled()) FreeNodeFeedbackReport.from(history.remoteCandidateId, delayMillis) else null
        val updated = previous.copy(
            version = AppConfig.FREE_NODE_STATE_VERSION,
            entries = previous.entries + (fingerprint to FreeNodeScoring.recordProbe(history, delayMillis, now)),
        )
        saveHistory(FreeNodeScoring.select(updated, now))
        return report
    }

    private fun ensurePolicyGroup(context: Context) {
        val storedId = MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_POLICY_GROUP_ID).orEmpty()
        val storedProfile = storedId.takeIf { it.isNotEmpty() }?.let(MmkvManager::decodeServerConfig)
        if (storedProfile?.policyGroupSubscriptionId == AppConfig.FREE_NODE_SUBSCRIPTION_ID) {
            if (storedProfile.policyGroupAutoSelection != true) {
                MmkvManager.encodeServerConfig(storedId, storedProfile.copy(policyGroupAutoSelection = true))
            }
            return
        }
        val profile = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            remarks = context.getString(R.string.free_node_policy_group_name)
            policyGroupSubscriptionId = AppConfig.FREE_NODE_SUBSCRIPTION_ID
            policyGroupTestOutbounds = true
            policyGroupType = BalancerStrategyType.LEAST_PING.policyGroupTypeValue
            policyGroupAutoSelection = true
        }
        val id = MmkvManager.encodeServerConfig("", profile)
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_POLICY_GROUP_ID, id)
    }

    private fun restoreSelection(selectedBefore: String?) {
        if (selectedBefore.isNullOrBlank()) {
            MmkvManager.clearSelectServer()
        } else {
            MmkvManager.setSelectServer(selectedBefore)
        }
    }

    private fun ensureFingerprintMigration() {
        if (MmkvManager.decodeSettingsInt(AppConfig.PREF_FREE_NODE_STATE_VERSION, 0) >= AppConfig.FREE_NODE_STATE_VERSION) return
        fingerprintKey()
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_STATE_VERSION, AppConfig.FREE_NODE_STATE_VERSION)
    }

    private fun loadHistory(): FreeNodeHistoryState =
        MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_HISTORY)
            ?.let { JsonUtil.fromJsonSafe(it, FreeNodeHistoryState::class.java) }
            ?: FreeNodeHistoryState()

    private fun saveHistory(state: FreeNodeHistoryState) {
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_HISTORY, JsonUtil.toJson(state))
    }

    private fun fingerprintKey(): ByteArray {
        val existing = MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_FINGERPRINT_KEY)
        if (!existing.isNullOrEmpty()) return Base64.decode(existing, Base64.NO_WRAP)
        return ByteArray(32).also { key ->
            SecureRandom().nextBytes(key)
            MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_FINGERPRINT_KEY, Base64.encodeToString(key, Base64.NO_WRAP))
        }
    }
}
