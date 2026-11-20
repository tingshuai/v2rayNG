package com.v2ray.ang.handler

import com.v2ray.ang.AppConfig
import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.enums.BalancerStrategyType
import com.v2ray.ang.enums.EConfigType

/** Owns the single internal policy profile used to connect an ordinary subscription as a group. */
object SubscriptionAutoSelectionManager {
    fun ensurePolicyGroup(subscriptionId: String): String? {
        if (subscriptionId.isBlank() || subscriptionId == AppConfig.FREE_NODE_SUBSCRIPTION_ID) return null

        val profile = MmkvManager.decodeServerConfig(AppConfig.SUBSCRIPTION_AUTO_SELECTION_PROFILE_GUID)
            ?.copy(
                configType = EConfigType.POLICYGROUP,
                policyGroupSubscriptionId = subscriptionId,
                policyGroupTestOutbounds = true,
                policyGroupType = BalancerStrategyType.LEAST_PING.policyGroupTypeValue,
                policyGroupAutoSelection = true,
            )
            ?: newProfile(subscriptionId)
        MmkvManager.encodeServerConfig(AppConfig.SUBSCRIPTION_AUTO_SELECTION_PROFILE_GUID, profile)
        return AppConfig.SUBSCRIPTION_AUTO_SELECTION_PROFILE_GUID
    }

    fun isInternalPolicyGroup(guid: String, profile: ProfileItem): Boolean =
        profile.configType == EConfigType.POLICYGROUP &&
            (profile.policyGroupAutoSelection == true ||
                guid == MmkvManager.decodeSettingsString(AppConfig.PREF_FREE_NODE_POLICY_GROUP_ID))

    internal fun newProfile(subscriptionId: String): ProfileItem =
        ProfileItem.create(EConfigType.POLICYGROUP).apply {
            remarks = "__subscription_auto_selection__"
            policyGroupSubscriptionId = subscriptionId
            policyGroupTestOutbounds = true
            policyGroupType = BalancerStrategyType.LEAST_PING.policyGroupTypeValue
            policyGroupAutoSelection = true
        }
}
