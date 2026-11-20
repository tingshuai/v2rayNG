package com.v2ray.ang.service

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class RealPingProgressTest {
    @Test
    fun progressUsesCompletedAndTotalCounts() {
        assertEquals("11 / 12", formatRealPingProgress(11, 12))
        assertEquals("12 / 12", formatRealPingProgress(12, 12))
    }
}
