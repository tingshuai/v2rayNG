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
}
