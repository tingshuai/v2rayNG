package com.v2ray.ang.handler

import com.v2ray.ang.dto.entities.ProfileItem

data class FreeNodeHistory(
    val lastSuccessAt: Long? = null,
    val lastFailureAt: Long? = null,
    val successCount: Int = 0,
    val failureCount: Int = 0,
    val consecutiveFailures: Int = 0,
    val latencyEwmaMillis: Double? = null,
    val jitterEwmaMillis: Double? = null,
    val isolatedUntil: Long? = null,
    val challengerWins: Int = 0,
)

data class FreeNodeHistoryState(
    val version: Int = 2,
    val entries: Map<String, FreeNodeHistory> = emptyMap(),
    val selectedFingerprint: String? = null,
)

/** Keeps only history for the current subscription replacement, keyed by a secret-safe fingerprint. */
object FreeNodeHistoryMigrator {
    fun migrate(
        state: FreeNodeHistoryState,
        profiles: Collection<ProfileItem>,
        fingerprint: (ProfileItem) -> String,
    ): FreeNodeHistoryState {
        val entries = profiles.associate { profile ->
            val key = fingerprint(profile)
            key to (state.entries[key] ?: FreeNodeHistory())
        }
        return state.copy(
            version = 2,
            entries = entries,
            selectedFingerprint = state.selectedFingerprint?.takeIf(entries::containsKey),
        )
    }
}
