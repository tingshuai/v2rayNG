package com.v2ray.ang.handler

import android.content.Context
import android.util.Base64
import com.v2ray.ang.AppConfig
import com.v2ray.ang.R
import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.dto.entities.SubscriptionItem
import com.v2ray.ang.enums.EConfigType
import com.v2ray.ang.util.JsonUtil
import java.security.SecureRandom

/** Owns the opt-in free-node subscription and its locally created policy group. */
object FreeNodeManager {
    fun isEnabled(): Boolean = MmkvManager.decodeSettingsBool(AppConfig.PREF_FREE_NODE_ENABLED, false)

    fun isDedicatedSubscription(subscriptionId: String): Boolean =
        subscriptionId == AppConfig.FREE_NODE_SUBSCRIPTION_ID

    fun enable(context: Context) {
        val selectedBefore = MmkvManager.getSelectServer()
        val existingSubscription = MmkvManager.decodeSubscription(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
        if (existingSubscription == null) {
            MmkvManager.encodeSubscription(
                AppConfig.FREE_NODE_SUBSCRIPTION_ID,
                SubscriptionItem(
                    remarks = context.getString(R.string.free_node_subscription_name),
                    url = AppConfig.FREE_NODE_SUBSCRIPTION_URL,
                    autoUpdate = true,
                ),
            )
        }
        try {
            ensurePolicyGroup(context)
            restoreSelection(selectedBefore)
            ensureFingerprintMigration()
            MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_RISK_NOTICE_VERSION, AppConfig.FREE_NODE_RISK_NOTICE_VERSION)
            MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_ENABLED, true)
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
        MmkvManager.removeSubscription(AppConfig.FREE_NODE_SUBSCRIPTION_ID)
    }

    fun fingerprint(profile: ProfileItem): String {
        return FreeNodeFingerprint.create(profile, fingerprintKey())
    }

    fun migrateHistory(profiles: Collection<ProfileItem>) {
        val previous = MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_HISTORY)
            ?.let { JsonUtil.fromJsonSafe(it, FreeNodeHistoryState::class.java) }
            ?: FreeNodeHistoryState()
        val migrated = FreeNodeHistoryMigrator.migrate(previous, profiles, ::fingerprint)
        MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_HISTORY, JsonUtil.toJson(migrated))
    }

    private fun ensurePolicyGroup(context: Context) {
        val storedId = MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_POLICY_GROUP_ID).orEmpty()
        if (storedId.isNotEmpty() && MmkvManager.decodeServerConfig(storedId)?.policyGroupSubscriptionId == AppConfig.FREE_NODE_SUBSCRIPTION_ID) return
        val profile = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            remarks = context.getString(R.string.free_node_policy_group_name)
            policyGroupSubscriptionId = AppConfig.FREE_NODE_SUBSCRIPTION_ID
            policyGroupTestOutbounds = true
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

    private fun fingerprintKey(): ByteArray {
        val existing = MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_FINGERPRINT_KEY)
        if (!existing.isNullOrEmpty()) return Base64.decode(existing, Base64.NO_WRAP)
        return ByteArray(32).also { key ->
            SecureRandom().nextBytes(key)
            MmkvManager.encodeSettings(AppConfig.PREF_FREE_NODE_FINGERPRINT_KEY, Base64.encodeToString(key, Base64.NO_WRAP))
        }
    }
}
