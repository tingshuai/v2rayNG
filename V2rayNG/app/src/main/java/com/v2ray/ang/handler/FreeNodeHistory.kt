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
    private const val RETAINED_SUCCESS_MILLIS = 7 * 24 * 60 * 60 * 1000L
    private const val MAX_RETAINED_PROFILES = 16

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

    fun recentlyValidatedProfiles(
        state: FreeNodeHistoryState,
        existingProfiles: Collection<ProfileItem>,
        incomingProfiles: Collection<ProfileItem>,
        fingerprint: (ProfileItem) -> String,
        now: Long,
    ): List<ProfileItem> {
        val incoming = incomingProfiles.mapTo(mutableSetOf(), fingerprint)
        return existingProfiles.asSequence()
            .mapNotNull { profile ->
                val key = fingerprint(profile)
                val history = state.entries[key] ?: return@mapNotNull null
                val recent = history.lastSuccessAt?.let { now - it in 0..RETAINED_SUCCESS_MILLIS } == true
                val isolated = history.isolatedUntil?.let { it > now } == true
                if (key !in incoming && recent && !isolated) profile else null
            }
            .sortedByDescending { state.entries[fingerprint(it)]?.lastSuccessAt ?: 0L }
            .take(MAX_RETAINED_PROFILES)
            .toList()
    }

    fun isSelectable(history: FreeNodeHistory?, now: Long): Boolean =
        history != null &&
            history.successCount > 0 &&
            history.lastSuccessAt?.let { now >= it && now - it <= RETAINED_SUCCESS_MILLIS } == true &&
            history.isolatedUntil == null
}
