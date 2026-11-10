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
}
