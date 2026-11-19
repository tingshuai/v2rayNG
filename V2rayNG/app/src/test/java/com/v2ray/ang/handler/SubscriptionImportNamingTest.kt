package com.v2ray.ang.handler

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SubscriptionImportNamingTest {
    @Test
    fun usesImportedFragmentAndAddsUniqueSuffixWhenNeeded() {
        assertEquals("手动导入", AngConfigManager.generateUniqueSubscriptionName("手动导入", emptySet()))
        assertEquals(
            "手动导入 3",
            AngConfigManager.generateUniqueSubscriptionName("手动导入", setOf("手动导入", "手动导入 2")),
        )
    }

    @Test
    fun blankImportedNameFallsBackToGeneratedName() {
        assertEquals("手动导入", AngConfigManager.generateUniqueSubscriptionName(" ", emptySet()))
    }
}
