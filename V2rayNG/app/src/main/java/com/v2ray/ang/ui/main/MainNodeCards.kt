package com.v2ray.ang.ui.main

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.v2ray.ang.R
import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.dto.LocateTarget
import com.v2ray.ang.dto.entities.SubscriptionUsageInfo
import com.v2ray.ang.ui.compose.LocalDarkTheme
import java.util.Locale

internal enum class LatencyTone { UNKNOWN, FAST, MODERATE, SLOW, FAILED }

internal fun latencyTone(delayMillis: Long): LatencyTone = when {
    delayMillis < 0L -> LatencyTone.FAILED
    delayMillis == 0L -> LatencyTone.UNKNOWN
    delayMillis <= 200L -> LatencyTone.FAST
    delayMillis <= 500L -> LatencyTone.MODERATE
    else -> LatencyTone.SLOW
}

@Composable
internal fun NodeCardsContent(
    groupId: String,
    mainViewModel: MainViewModel,
    selectedGuid: String?,
    locateTarget: LocateTarget?,
    onLocateHandled: () -> Unit,
    usageInfo: SubscriptionUsageInfo?,
    groupTitle: String,
    onOpenLink: (String) -> Unit,
    onRefresh: () -> Unit,
    selectedCountryCode: String?,
    onFilterCountry: (String?) -> Unit,
    onSelect: (String) -> Unit,
    onEdit: (String, ProfileItem) -> Unit,
    onShare: (String, ProfileItem) -> Unit,
    onRemove: (String, String) -> Unit,
    isTesting: Boolean,
    testProgress: String?,
    onTest: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val flow = remember(groupId) { mainViewModel.serverGroupState(groupId) }
    val state by flow.collectAsStateWithLifecycle()
    val availableCountries = state.countryByGuid.values.distinct().sorted()
    val visibleRows = if (selectedCountryCode == null) state.rows else state.rows.filter {
        state.countryByGuid[it.guid] == selectedCountryCode
    }
    val dark = LocalDarkTheme.current
    val selectedBackground = if (dark) Color(0xFF19314D) else Color(0xFFE8F1FF)
    val selectedBorder = if (dark) Color(0xFF7AB8FF) else Color(0xFF3F82DB)
    val listState = rememberLazyListState()
    LaunchedEffect(locateTarget, visibleRows, usageInfo) {
        if (locateTarget?.groupId == groupId) {
            val index = visibleRows.indexOfFirst { it.guid == locateTarget.serverGuid }
            if (index >= 0) listState.animateScrollToItem(index)
            onLocateHandled()
        }
    }
    Column(modifier = modifier.imePadding()) {
        if (usageInfo != null) {
            SubscriptionInfoCard(info = usageInfo, fallbackTitle = groupTitle, onOpenLink = onOpenLink, onRefresh = onRefresh)
        }
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            val measured = visibleRows.count { it.testDelayMillis != 0L }
            Text(stringResource(R.string.fn21_node_summary, visibleRows.size, measured), style = MaterialTheme.typography.bodySmall)
            if (isTesting) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                    Text(testProgress?.let { stringResource(R.string.connection_running_task_left, it) }
                        ?: stringResource(R.string.connection_test_testing), style = MaterialTheme.typography.bodySmall)
                }
            } else {
                TextButton(onClick = onTest, enabled = !isTesting) {
                    Text(stringResource(R.string.connection_test_pending))
                }
            }
        }
        Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(
                selected = selectedCountryCode == null,
                onClick = { onFilterCountry(null) },
                label = { Text(stringResource(R.string.filter_config_all)) },
            )
            availableCountries.forEach { code ->
                FilterChip(
                    selected = selectedCountryCode == code,
                    onClick = { onFilterCountry(code) },
                    label = { Text(Locale.Builder().setRegion(code).build().getDisplayCountry(Locale.getDefault())) },
                )
            }
        }
        LazyColumn(state = listState, modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        items(visibleRows, key = { it.guid }) { row ->
            val isSelected = row.guid == selectedGuid
            val status = when {
                row.testDelayMillis > 0L -> stringResource(R.string.fn21_healthy)
                row.testDelayMillis < 0L -> stringResource(R.string.fn21_failed)
                else -> stringResource(R.string.fn21_untested)
            }
            val semanticLabel = stringResource(R.string.fn21_node_semantics, row.remarks, row.typeDescription, status)
            val statusColor = when {
                row.testDelayMillis > 0L -> MaterialTheme.colorScheme.tertiary
                row.testDelayMillis < 0L -> MaterialTheme.colorScheme.error
                else -> MaterialTheme.colorScheme.onSurfaceVariant
            }
            val delayColor = when (latencyTone(row.testDelayMillis)) {
                LatencyTone.FAST -> MaterialTheme.colorScheme.tertiary
                LatencyTone.MODERATE -> MaterialTheme.colorScheme.secondary
                LatencyTone.SLOW, LatencyTone.FAILED -> MaterialTheme.colorScheme.error
                LatencyTone.UNKNOWN -> MaterialTheme.colorScheme.onSurfaceVariant
            }
            Card(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp)
                    .semantics { selected = isSelected }
                    .clickable(role = Role.Button, onClickLabel = semanticLabel) { onSelect(row.guid) },
                colors = CardDefaults.cardColors(containerColor = if (isSelected) selectedBackground else MaterialTheme.colorScheme.surface),
                border = BorderStroke(1.dp, if (isSelected) selectedBorder else MaterialTheme.colorScheme.outlineVariant),
            ) {
                Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(Modifier.padding(end = 10.dp).size(24.dp).background(
                            if (row.testDelayMillis == 0L) MaterialTheme.colorScheme.surfaceContainerHigh else statusColor,
                            CircleShape,
                        ), contentAlignment = Alignment.Center) {
                            Text(
                                if (row.testDelayMillis > 0L) "✓" else if (row.testDelayMillis < 0L) "×" else "?",
                                color = if (row.testDelayMillis == 0L) statusColor else if (row.testDelayMillis > 0L) MaterialTheme.colorScheme.onTertiary else MaterialTheme.colorScheme.onError,
                                style = MaterialTheme.typography.labelSmall,
                            )
                        }
                        Text(row.remarks, Modifier.weight(1f), style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(status, style = MaterialTheme.typography.labelSmall, color = statusColor)
                        NodeCardMenu(row, onEdit, onShare, onRemove)
                    }
                    Spacer(Modifier.size(4.dp))
                    Row(horizontalArrangement = Arrangement.SpaceBetween, modifier = Modifier.fillMaxWidth()) {
                        Text(row.typeDescription, Modifier.weight(1.2f), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(
                            state.countryByGuid[row.guid]?.let { Locale.Builder().setRegion(it).build().getDisplayCountry(Locale.getDefault()) }
                                ?: stringResource(R.string.fn21_unknown_region),
                            Modifier.weight(1f),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Text(if (row.testDelayMillis > 0L) stringResource(R.string.server_test_delay_value, row.testDelayMillis) else "", Modifier.weight(0.8f), style = MaterialTheme.typography.bodySmall, color = delayColor, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
        }
        }
    }
}
