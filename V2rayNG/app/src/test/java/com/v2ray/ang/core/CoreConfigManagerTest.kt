package com.v2ray.ang.core

import com.v2ray.ang.AppConfig
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class CoreConfigManagerTest {
    @Test
    fun missingBalancerFailsOnlyForFreeNodePolicyGroup() {
        assertTrue(CoreConfigManager.isMissingRequiredFreeNodeBalancer(true, emptySet()))
        assertFalse(CoreConfigManager.isMissingRequiredFreeNodeBalancer(false, emptySet()))
        assertFalse(CoreConfigManager.isMissingRequiredFreeNodeBalancer(true, setOf(AppConfig.TAG_BALANCER)))
    }

    @Test
    fun rejectsAutomaticSubscriptionGroupsWithoutVerifiedMembers() {
        assertTrue(CoreConfigManager.isMissingRequiredAutomaticPolicyGroupBalancer(true, emptySet()))
        assertFalse(CoreConfigManager.isMissingRequiredAutomaticPolicyGroupBalancer(false, emptySet()))
        assertFalse(CoreConfigManager.isMissingRequiredAutomaticPolicyGroupBalancer(true, setOf(AppConfig.TAG_BALANCER)))
    }

    @Test
    fun automaticSubscriptionGroupIncludesOnlyLocallyPassingMembers() {
        assertTrue(includePolicyGroupMember(false, true, 42L))
        assertFalse(includePolicyGroupMember(false, true, -1L))
        assertFalse(includePolicyGroupMember(false, true, null))
        assertTrue(includePolicyGroupMember(true, true, -1L))
        assertTrue(includePolicyGroupMember(false, false, null))
    }
}
