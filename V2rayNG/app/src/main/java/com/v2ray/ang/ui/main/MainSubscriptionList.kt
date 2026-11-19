package com.v2ray.ang.ui.main

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.v2ray.ang.AppConfig
import com.v2ray.ang.R
import com.v2ray.ang.dto.GroupMapItem
import com.v2ray.ang.ui.compose.LocalDarkTheme
import com.v2ray.ang.util.Utils

internal data class SubscriptionCounts(val total: Int, val healthy: Int, val failed: Int, val untested: Int)

internal fun subscriptionCounts(state: ServerGroupUiState): SubscriptionCounts {
    val healthy = state.servers.count { it.testDelayMillis > 0L }
    val failed = state.servers.count { it.testDelayMillis < 0L }
    return SubscriptionCounts(state.servers.size, healthy, failed, state.servers.size - healthy - failed)
}

internal fun orderedSubscriptions(groups: List<GroupMapItem>): List<GroupMapItem> =
    groups.sortedWith(compareBy<GroupMapItem> { it.id != AppConfig.FREE_NODE_SUBSCRIPTION_ID }
        .thenBy { groups.indexOf(it) })

@Composable
internal fun SubscriptionListContent(
    groups: List<GroupMapItem>,
    mainViewModel: MainViewModel,
    selectedGroupId: String,
    onSelect: (String) -> Unit,
    onOpen: (String) -> Unit,
    onManage: () -> Unit,
    onUpdate: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val ordered = remember(groups) { orderedSubscriptions(groups) }
    val dark = LocalDarkTheme.current
    val selectedBackground = if (dark) Color(0xFF19314D) else Color(0xFFE8F1FF)
    val selectedBorder = if (dark) Color(0xFF7AB8FF) else Color(0xFF3F82DB)
    LazyColumn(modifier = modifier, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items(ordered, key = { it.id }) { group ->
            val flow = remember(group.id) { mainViewModel.serverGroupState(group.id) }
            val state by flow.collectAsStateWithLifecycle()
            val counts = subscriptionCounts(state)
            val isFree = group.id == AppConfig.FREE_NODE_SUBSCRIPTION_ID
            val title = when (group.id) {
                AppConfig.DEFAULT_SUBSCRIPTION_ID -> stringResource(R.string.fn21_manual_import)
                AppConfig.FREE_NODE_SUBSCRIPTION_ID -> stringResource(R.string.free_node_subscription_name)
                else -> group.remarks
            }
            val isSelected = group.id == selectedGroupId
            val label = stringResource(R.string.fn21_subscription_summary, title, counts.total, counts.healthy, counts.failed, counts.untested)
            Card(
                onClick = { onSelect(group.id) },
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp)
                    .semantics { selected = isSelected; contentDescription = label },
                colors = CardDefaults.cardColors(containerColor = if (isSelected) selectedBackground else MaterialTheme.colorScheme.surface),
                border = BorderStroke(1.dp, if (isSelected) selectedBorder else MaterialTheme.colorScheme.outlineVariant),
            ) {
                Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(Modifier.size(30.dp).background(MaterialTheme.colorScheme.secondaryContainer, CircleShape), contentAlignment = Alignment.Center) {
                            Text(if (isSelected) "✓" else title.take(1).uppercase(), style = MaterialTheme.typography.labelMedium)
                        }
                        Spacer(Modifier.size(10.dp))
                        Text(title, Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (isFree) {
                            Text(stringResource(R.string.fn21_free_fixed), style = MaterialTheme.typography.labelSmall, color = selectedBorder)
                        } else if (group.id.isNotEmpty()) {
                            SubscriptionCardMenu(onUpdate = { onUpdate(group.id) }, onManage = onManage)
                        }
                    }
                    Text(
                        when {
                            isFree -> stringResource(R.string.fn21_free_subtitle)
                            group.id == AppConfig.DEFAULT_SUBSCRIPTION_ID -> stringResource(R.string.fn21_manual_subtitle)
                            else -> stringResource(R.string.fn21_user_subtitle)
                        },
                        Modifier.padding(top = 6.dp),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    HorizontalDivider(Modifier.padding(vertical = 9.dp))
                    Text(stringResource(R.string.fn21_counts, counts.total, counts.healthy, counts.failed, counts.untested), style = MaterialTheme.typography.bodySmall)
                    val latestTestAt = state.servers.maxOfOrNull { it.lastTestAt } ?: 0L
                    if (latestTestAt > 0L) {
                        Text(stringResource(R.string.fn21_last_test, Utils.formatTimestamp(latestTestAt, "yyyy-MM-dd HH:mm")), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                        TextButton(onClick = { onOpen(group.id) }) {
                            Text(stringResource(R.string.fn21_open_nodes))
                            Text("  ›", color = MaterialTheme.colorScheme.primary)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SubscriptionCardMenu(onUpdate: () -> Unit, onManage: () -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { expanded = true }, modifier = Modifier.size(32.dp)) {
            Icon(painterResource(R.drawable.ic_more_vert_24dp), contentDescription = stringResource(R.string.acc_more), modifier = Modifier.size(20.dp))
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            DropdownMenuItem(text = { Text(stringResource(R.string.title_sub_update)) }, onClick = { expanded = false; onUpdate() })
            DropdownMenuItem(text = { Text(stringResource(R.string.title_sub_setting)) }, onClick = { expanded = false; onManage() })
        }
    }
}
