package com.v2ray.ang.ui.main

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

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
}
