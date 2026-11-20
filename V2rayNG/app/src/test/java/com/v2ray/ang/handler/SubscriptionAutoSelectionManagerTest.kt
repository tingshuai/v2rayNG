package com.v2ray.ang.handler

import com.v2ray.ang.AppConfig
import com.v2ray.ang.enums.BalancerStrategyType
import com.v2ray.ang.enums.EConfigType
import com.v2ray.ang.util.JsonUtil
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class SubscriptionAutoSelectionManagerTest {
    @Test
    fun createsAutomaticLeastPingPolicyGroupForSelectedSubscription() {
        val profile = SubscriptionAutoSelectionManager.newProfile("subscription-b")

        assertEquals(EConfigType.POLICYGROUP, profile.configType)
        assertEquals("subscription-b", profile.policyGroupSubscriptionId)
        assertEquals(BalancerStrategyType.LEAST_PING.policyGroupTypeValue, profile.policyGroupType)
        assertTrue(profile.policyGroupTestOutbounds == true)
        assertTrue(profile.policyGroupAutoSelection == true)
    }

    @Test
    fun doesNotReplaceTheDedicatedFreeNodeGroup() {
        assertNull(SubscriptionAutoSelectionManager.ensurePolicyGroup(AppConfig.FREE_NODE_SUBSCRIPTION_ID))
        assertNull(SubscriptionAutoSelectionManager.ensurePolicyGroup(""))
    }

    @Test
    fun olderProfilesWithoutTheInternalMarkerRemainOrdinaryProfiles() {
        val profile = JsonUtil.fromJsonSafe("""{"configType":"POLICYGROUP"}""", com.v2ray.ang.dto.entities.ProfileItem::class.java)

        assertEquals(EConfigType.POLICYGROUP, profile?.configType)
        assertNull(profile?.policyGroupAutoSelection)
        assertFalse(profile?.policyGroupAutoSelection == true)
    }
}
