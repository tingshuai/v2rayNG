package com.v2ray.ang.util

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LogUtilTest {
    @Test
    fun diagnosticIdIsStableAndDoesNotExposeRawValue() {
        val rawId = "subscription-guid-example"
        val first = LogUtil.diagnosticId(rawId)

        assertEquals(first, LogUtil.diagnosticId(rawId))
        assertEquals(16, first.length)
        assertTrue(first.matches(Regex("[a-f0-9]{16}")))
        assertNotEquals(rawId, first)
    }

    @Test
    fun diagnosticIdHandlesMissingValues() {
        assertEquals("none", LogUtil.diagnosticId(null))
        assertEquals("none", LogUtil.diagnosticId(" "))
    }
}
