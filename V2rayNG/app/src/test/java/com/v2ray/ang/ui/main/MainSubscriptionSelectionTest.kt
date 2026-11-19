package com.v2ray.ang.ui.main

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.enums.EConfigType

class MainSubscriptionSelectionTest {
    @Test
    fun selectingSubscriptionUpdatesTheConnectionTarget() {
        val initial = MainUiState(selectedSubscriptionId = "first", selectedGroupId = "first")

        val selected = initial.withSelectedSubscription("second")

        assertEquals("second", selected.selectedSubscriptionId)
        assertEquals("second", selected.selectedGroupId)
        assertTrue(selected.showingSubscriptions)
    }

    @Test
    fun openingAnotherNodeListKeepsTheSelectedSubscription() {
        val initial = MainUiState(selectedSubscriptionId = "selected", selectedGroupId = "selected")

        val opened = initial.withOpenedSubscription("browsed")

        assertEquals("selected", opened.selectedSubscriptionId)
        assertEquals("browsed", opened.selectedGroupId)
        assertFalse(opened.showingSubscriptions)
    }

    @Test
    fun selectedSubscriptionConnectionTargetIsItsPolicyGroup() {
        val selected = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            policyGroupSubscriptionId = "subscription-b"
        }

        assertTrue(selected.configType == EConfigType.POLICYGROUP)
        assertEquals("subscription-b", selected.policyGroupSubscriptionId)
    }

    @Test
    fun changesConnectionTargetWhenSelectionIsNotItsPolicyGroup() {
        val current = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            policyGroupSubscriptionId = "subscription-a"
        }
        val selected = ProfileItem.create(EConfigType.POLICYGROUP).apply {
            policyGroupSubscriptionId = "subscription-b"
        }

        assertTrue(shouldReplaceConnectionTarget(current, "subscription-b"))
        assertFalse(shouldReplaceConnectionTarget(selected, "subscription-b"))
        assertTrue(shouldReplaceConnectionTarget(null, "subscription-b"))
    }
}
