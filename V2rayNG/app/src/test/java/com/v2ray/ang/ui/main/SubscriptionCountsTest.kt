package com.v2ray.ang.ui.main

import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.dto.entities.ServersCache
import com.v2ray.ang.dto.GroupMapItem
import com.v2ray.ang.AppConfig
import com.v2ray.ang.enums.EConfigType
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class SubscriptionCountsTest {
    @Test
    fun separatesRealSuccessFailureAndUntested() {
        val state = ServerGroupUiState(
            servers = listOf(86L, -1L, 0L).mapIndexed { index, delay ->
                ServersCache(guid = "server-$index", profile = ProfileItem(configType = EConfigType.VMESS), testDelayMillis = delay)
            }
        )
        assertEquals(SubscriptionCounts(3, 1, 1, 1), subscriptionCounts(state))
        assertEquals(SubscriptionCounts(0, 0, 0, 0), subscriptionCounts(ServerGroupUiState()))
    }

    @Test
    fun freeSubscriptionComesFirstWithoutReorderingOtherGroups() {
        val groups = listOf(
            GroupMapItem("first", "First"),
            GroupMapItem(AppConfig.FREE_NODE_SUBSCRIPTION_ID, "Free"),
            GroupMapItem("last", "Last"),
        )
        assertEquals(listOf(AppConfig.FREE_NODE_SUBSCRIPTION_ID, "first", "last"), orderedSubscriptions(groups).map { it.id })
    }
}
