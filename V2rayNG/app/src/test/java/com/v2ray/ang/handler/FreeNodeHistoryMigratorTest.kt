package com.v2ray.ang.handler

import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.enums.EConfigType
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class FreeNodeHistoryMigratorTest {
    @Test
    fun `retains matching history and removes absent candidates`() {
        val state = FreeNodeHistoryState(entries = mapOf("same" to FreeNodeHistory(successCount = 2), "gone" to FreeNodeHistory(failureCount = 1)))
        val profiles = listOf(ProfileItem.create(EConfigType.VLESS), ProfileItem.create(EConfigType.TROJAN))
        val migrated = FreeNodeHistoryMigrator.migrate(state, profiles) { if (it.configType == EConfigType.VLESS) "same" else "new" }
        assertEquals(2, migrated.entries["same"]?.successCount)
        assertEquals(0, migrated.entries["new"]?.failureCount)
        assertEquals(setOf("same", "new"), migrated.entries.keys)
    }

    @Test
    fun `keeps only recent successful non-isolated profiles missing from new feed`() {
        val now = 10_000_000L
        val profiles = listOf(
            ProfileItem.create(EConfigType.VLESS),
            ProfileItem.create(EConfigType.TROJAN),
            ProfileItem.create(EConfigType.SHADOWSOCKS),
            ProfileItem.create(EConfigType.VMESS),
        )
        val ids = mapOf(
            EConfigType.VLESS to "healthy",
            EConfigType.TROJAN to "incoming",
            EConfigType.SHADOWSOCKS to "isolated",
            EConfigType.VMESS to "stale",
        )
        val state = FreeNodeHistoryState(entries = mapOf(
            "healthy" to FreeNodeHistory(lastSuccessAt = now - 1_000, successCount = 2),
            "incoming" to FreeNodeHistory(lastSuccessAt = now - 1_000, successCount = 2),
            "isolated" to FreeNodeHistory(lastSuccessAt = now - 1_000, successCount = 2, isolatedUntil = now + 5_000),
            "stale" to FreeNodeHistory(lastSuccessAt = now - 8 * 24 * 60 * 60 * 1000L, successCount = 2),
        ))

        val retained = FreeNodeHistoryMigrator.recentlyValidatedProfiles(
            state,
            profiles,
            listOf(ProfileItem.create(EConfigType.TROJAN)),
            { ids.getValue(it.configType) },
            now,
        )

        assertEquals(listOf(EConfigType.VLESS), retained.map { it.configType })
    }

    @Test
    fun `only locally successful and non-isolated candidates enter policy group`() {
        val now = 10_000L
        assertEquals(false, FreeNodeHistoryMigrator.isSelectable(null, now))
        assertEquals(false, FreeNodeHistoryMigrator.isSelectable(FreeNodeHistory(), now))
        assertEquals(false, FreeNodeHistoryMigrator.isSelectable(FreeNodeHistory(successCount = 1, lastSuccessAt = now, isolatedUntil = now + 1), now))
        assertEquals(false, FreeNodeHistoryMigrator.isSelectable(FreeNodeHistory(successCount = 1, lastSuccessAt = now, isolatedUntil = now), now))
        assertEquals(true, FreeNodeHistoryMigrator.isSelectable(FreeNodeHistory(successCount = 1, lastSuccessAt = now), now))
        assertEquals(false, FreeNodeHistoryMigrator.isSelectable(FreeNodeHistory(successCount = 1, lastSuccessAt = now - 8 * 24 * 60 * 60 * 1000L), now))
    }

    @Test
    fun `server-revoked candidate is not retained from previous feed`() {
        val now = 10_000L
        val candidateId = "a".repeat(64)
        val profile = ProfileItem.create(EConfigType.VLESS)
        val state = FreeNodeHistoryState(
            entries = mapOf("local-fingerprint" to FreeNodeHistory(lastSuccessAt = now, successCount = 1, remoteCandidateId = candidateId)),
            revokedCandidateIds = setOf(candidateId),
        )
        assertEquals(
            emptyList<ProfileItem>(),
            FreeNodeHistoryMigrator.recentlyValidatedProfiles(state, listOf(profile), emptyList(), { "local-fingerprint" }, now),
        )
    }

    @Test
    fun `revocation identifies local profiles for physical removal`() {
        val revokedId = "a".repeat(64)
        val retainedId = "b".repeat(64)
        val state = FreeNodeHistoryState(
            entries = mapOf(
                "revoked-fingerprint" to FreeNodeHistory(remoteCandidateId = revokedId),
                "retained-fingerprint" to FreeNodeHistory(remoteCandidateId = retainedId),
            ),
        )

        assertEquals(
            setOf("revoked-fingerprint"),
            FreeNodeHistoryMigrator.revokedFingerprints(state, setOf(revokedId)),
        )
    }
}
