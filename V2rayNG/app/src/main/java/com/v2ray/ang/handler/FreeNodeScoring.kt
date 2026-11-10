package com.v2ray.ang.handler

/** Pure local scoring and hysteresis rules for the dedicated free-node subscription. */
object FreeNodeScoring {
    const val MAX_PROBES = 8
    private const val HEALTHY_SAMPLE_SIZE = 4
    private const val NEW_SAMPLE_SIZE = 2
    private const val RECOVERY_SAMPLE_SIZE = 2
    private const val FAILURE_ISOLATION_THRESHOLD = 3
    private const val ISOLATION_MILLIS = 30 * 60 * 1000L
    private const val EWMA_ALPHA = 0.35
    private const val CHALLENGER_ADVANTAGE = 800.0
    private const val CHALLENGER_WINS_REQUIRED = 2

    fun recordProbe(history: FreeNodeHistory, delayMillis: Long, now: Long): FreeNodeHistory {
        if (delayMillis <= 0) {
            val consecutiveFailures = history.consecutiveFailures + 1
            return history.copy(
                lastFailureAt = now,
                failureCount = history.failureCount + 1,
                consecutiveFailures = consecutiveFailures,
                isolatedUntil = if (consecutiveFailures >= FAILURE_ISOLATION_THRESHOLD) now + ISOLATION_MILLIS else history.isolatedUntil,
            )
        }

        val previousLatency = history.latencyEwmaMillis
        val latency = ewma(previousLatency, delayMillis.toDouble())
        val jitter = ewma(history.jitterEwmaMillis, kotlin.math.abs(delayMillis.toDouble() - (previousLatency ?: delayMillis.toDouble())))
        return history.copy(
            lastSuccessAt = now,
            successCount = history.successCount + 1,
            consecutiveFailures = 0,
            latencyEwmaMillis = latency,
            jitterEwmaMillis = jitter,
            isolatedUntil = null,
        )
    }

    fun select(state: FreeNodeHistoryState, now: Long): FreeNodeHistoryState {
        val eligible = state.entries.filterValues { it.isolatedUntil?.let(now::compareTo)?.let { comparison -> comparison >= 0 } != false }
            .filterValues { it.successCount > 0 }
        if (eligible.isEmpty()) return state

        val incumbent = state.selectedFingerprint?.takeIf(eligible::containsKey)
        val challenger = eligible.maxByOrNull { (_, history) -> score(history, now) }?.key ?: return state
        if (incumbent == null) return state.copy(selectedFingerprint = challenger)
        if (challenger == incumbent) return state.copy(entries = state.entries.mapValues { (fingerprint, history) ->
            if (fingerprint == incumbent && history.challengerWins != 0) history.copy(challengerWins = 0) else history
        })

        val incumbentScore = score(eligible.getValue(incumbent), now)
        val challengerScore = score(eligible.getValue(challenger), now)
        if (challengerScore < incumbentScore + CHALLENGER_ADVANTAGE) {
            return state.copy(entries = state.entries.mapValues { (fingerprint, history) ->
                if (fingerprint == challenger && history.challengerWins != 0) history.copy(challengerWins = 0) else history
            })
        }

        val wins = (state.entries.getValue(challenger).challengerWins + 1)
        val updatedEntries = state.entries.mapValues { (fingerprint, history) ->
            when (fingerprint) {
                challenger -> history.copy(challengerWins = wins)
                incumbent -> if (history.challengerWins != 0) history.copy(challengerWins = 0) else history
                else -> history
            }
        }
        return if (wins >= CHALLENGER_WINS_REQUIRED) {
            state.copy(entries = updatedEntries.mapValues { (fingerprint, history) ->
                if (fingerprint == challenger) history.copy(challengerWins = 0) else history
            }, selectedFingerprint = challenger)
        } else {
            state.copy(entries = updatedEntries)
        }
    }

    fun plan(state: FreeNodeHistoryState, now: Long): List<String> {
        val ranked = state.entries.entries.sortedWith(compareByDescending<Map.Entry<String, FreeNodeHistory>> { score(it.value, now) }.thenBy { it.key })
        val healthy = ranked.filter { (_, history) -> history.successCount > 0 && (history.isolatedUntil == null || history.isolatedUntil <= now) }
            .take(HEALTHY_SAMPLE_SIZE)
        val newCandidates = ranked.filter { (_, history) -> history.successCount == 0 && history.failureCount == 0 }
            .take(NEW_SAMPLE_SIZE)
        val recovered = ranked.filter { (_, history) -> history.isolatedUntil != null && history.isolatedUntil <= now }
            .take(RECOVERY_SAMPLE_SIZE)
        return (healthy + newCandidates + recovered).map { it.key }.distinct().take(MAX_PROBES)
    }

    fun score(history: FreeNodeHistory, now: Long): Double {
        if (history.isolatedUntil?.let { it > now } == true) return Double.NEGATIVE_INFINITY
        val attempts = history.successCount + history.failureCount
        val successRate = if (attempts == 0) 0.0 else history.successCount.toDouble() / attempts
        val stability = (history.successCount - history.consecutiveFailures * 2).coerceAtLeast(0)
        val latencyPenalty = history.latencyEwmaMillis ?: 1_500.0
        val jitterPenalty = history.jitterEwmaMillis ?: 500.0
        return successRate * 10_000 + stability * 300 - latencyPenalty - jitterPenalty * 0.5
    }

    private fun ewma(previous: Double?, sample: Double): Double = previous?.let { EWMA_ALPHA * sample + (1 - EWMA_ALPHA) * it } ?: sample
}
