package com.v2ray.ang.handler

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class FreeNodeScoringTest {
    @Test
    fun `one failure does not isolate or replace a healthy incumbent`() {
        val now = 1_000L
        val incumbent = FreeNodeHistory(successCount = 4, latencyEwmaMillis = 100.0, jitterEwmaMillis = 5.0)
        val challenger = FreeNodeHistory(successCount = 4, latencyEwmaMillis = 50.0, jitterEwmaMillis = 2.0)
        val failed = FreeNodeScoring.recordProbe(incumbent, -1, now)
        val selected = FreeNodeScoring.select(
            FreeNodeHistoryState(entries = mapOf("incumbent" to failed, "challenger" to challenger), selectedFingerprint = "incumbent"),
            now,
        )

        assertEquals("incumbent", selected.selectedFingerprint)
        assertNull(failed.isolatedUntil)
    }

    @Test
    fun `challenger must win repeatedly before it replaces incumbent`() {
        val now = 1_000L
        val incumbent = FreeNodeHistory(successCount = 4, latencyEwmaMillis = 900.0, jitterEwmaMillis = 100.0)
        val challenger = FreeNodeHistory(successCount = 4, latencyEwmaMillis = 50.0, jitterEwmaMillis = 2.0)
        val initial = FreeNodeHistoryState(entries = mapOf("incumbent" to incumbent, "challenger" to challenger), selectedFingerprint = "incumbent")

        val first = FreeNodeScoring.select(initial, now)
        val second = FreeNodeScoring.select(first, now + 1)

        assertEquals("incumbent", first.selectedFingerprint)
        assertEquals("challenger", second.selectedFingerprint)
    }

    @Test
    fun `third failure isolates candidate and expired isolation is sampled`() {
        val now = 1_000L
        val afterFailures = generateSequence(FreeNodeHistory()) { FreeNodeScoring.recordProbe(it, -1, now) }
            .drop(3)
            .first()
        val state = FreeNodeHistoryState(entries = mapOf("isolated" to afterFailures))

        assertEquals(3, afterFailures.consecutiveFailures)
        assertEquals(emptyList<String>(), FreeNodeScoring.plan(state, now))
        assertEquals(listOf("isolated"), FreeNodeScoring.plan(state, afterFailures.isolatedUntil!!))
    }

    @Test
    fun `probe plan remains bounded and includes new candidates`() {
        val entries = (1..12).associate { index ->
            "candidate-$index" to if (index <= 6) {
                FreeNodeHistory(successCount = index, latencyEwmaMillis = index.toDouble(), jitterEwmaMillis = 1.0)
            } else {
                FreeNodeHistory()
            }
        }

        val plan = FreeNodeScoring.plan(FreeNodeHistoryState(entries = entries), 1_000L)

        assertEquals(6, plan.size)
        assertEquals(2, plan.count { it in setOf("candidate-7", "candidate-8", "candidate-9", "candidate-10", "candidate-11", "candidate-12") })
    }
}
