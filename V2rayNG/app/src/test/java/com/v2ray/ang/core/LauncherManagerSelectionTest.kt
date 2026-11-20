package com.v2ray.ang.core

import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.enums.EConfigType
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LauncherManagerSelectionTest {
    @Test
    fun resolvesMissingOrMismatchedSubscriptionPolicyGroup() {
        val otherGroup = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            policyGroupSubscriptionId = "subscription-a"
        }

        assertTrue(shouldResolveSubscriptionGroupAtStart(null, null, "subscription-b"))
        assertTrue(shouldResolveSubscriptionGroupAtStart(null, otherGroup, "subscription-b"))
        assertFalse(shouldResolveSubscriptionGroupAtStart(null, otherGroup, "subscription-a"))
    }

    @Test
    fun preservesDirectNodeSelectionAndExplicitTaskerProfile() {
        val directNode = ProfileItem.create(EConfigType.VLESS).apply { subscriptionId = "subscription-a" }
        val policyGroup = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            policyGroupSubscriptionId = "subscription-a"
        }

        assertFalse(shouldResolveSubscriptionGroupAtStart(null, directNode, "subscription-a"))
        assertFalse(shouldResolveSubscriptionGroupAtStart("explicit-guid", policyGroup, "subscription-b"))
    }
}
