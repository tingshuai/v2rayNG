package com.v2ray.ang.ui.main

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MainBottomBarStatusTest {
    @Test
    fun activeTestTextTakesPriorityOverPreviousConnectionResult() {
        assertEquals(
            "Testing…",
            mainBottomStatusText(MainStatus.Testing, "Testing…", "Previous connection succeeded"),
        )
    }

    @Test
    fun latestConnectionResultRemainsVisibleAfterTestCompletes() {
        assertEquals(
            "Connection succeeded: 130 ms",
            mainBottomStatusText(MainStatus.Connected, "Connected", "Connection succeeded: 130 ms"),
        )
    }

    @Test
    fun currentStatusIsUsedWhenThereIsNoConnectionResult() {
        assertEquals(
            "Disconnected",
            mainBottomStatusText(MainStatus.Disconnected, "Disconnected", null),
        )
    }
}
