package com.v2ray.ang.handler

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class FreeNodeFeedbackReportTest {
    @Test
    fun `reports only opaque candidate id and coarse outcome buckets`() {
        val id = "c".repeat(64)
        assertEquals(FreeNodeFeedbackReport(id, "failure", "none"), FreeNodeFeedbackReport.from(id, 0))
        assertEquals("lt_100", FreeNodeFeedbackReport.from(id, 99)?.latency_bucket)
        assertEquals("100_300", FreeNodeFeedbackReport.from(id, 100)?.latency_bucket)
        assertEquals("300_1000", FreeNodeFeedbackReport.from(id, 300)?.latency_bucket)
        assertEquals("gte_1000", FreeNodeFeedbackReport.from(id, 1_000)?.latency_bucket)
    }

    @Test
    fun `rejects missing or malformed server candidate id`() {
        assertNull(FreeNodeFeedbackReport.from(null, 10))
        assertNull(FreeNodeFeedbackReport.from("not-an-id", 10))
    }
}
