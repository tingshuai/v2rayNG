package com.v2ray.ang.ui.main

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.v2ray.ang.R
import com.v2ray.ang.ui.compose.AppDivider
import kotlinx.coroutines.launch

@Composable
fun MainBottomBar(
    displayText: String,
    selectedServerLabel: String?,
    selectedServerDelayMillis: Long,
    connectionTestLabel: String?,
    connectionGroupLabel: String,
    connectionSource: ConnectionSource,
    status: MainStatus,
    isRunning: Boolean,
    onAction: (MainAction) -> Unit
) {
    val scope = rememberCoroutineScope()
    val rotationAnim = remember { Animatable(0f) }
    val borderRotationAnim = remember { Animatable(0f) }
    var isStartingConnection by remember { mutableStateOf(false) }
    val connectionIsTesting = status is MainStatus.Testing
    val connectionIsActive = connectionIsTesting || isStartingConnection
    val connectionIsHealthy = isRunning && !connectionIsTesting && (
        status is MainStatus.Connected || status is MainStatus.ConnectionTest && status.result.delayMillis >= 0L
    )
    val connectionFailed = isRunning && status is MainStatus.ConnectionTest && status.result.delayMillis < 0L
    val connectionButtonColor = when {
        connectionIsTesting -> MaterialTheme.colorScheme.surfaceContainerHigh
        connectionIsHealthy -> MaterialTheme.colorScheme.tertiary
        connectionFailed -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.surfaceContainerHigh
    }
    val connectionButtonContentColor = when {
        connectionIsTesting -> MaterialTheme.colorScheme.onSurfaceVariant
        connectionIsHealthy -> MaterialTheme.colorScheme.onTertiary
        connectionFailed -> MaterialTheme.colorScheme.onError
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    val edgeHighlightColors = listOf(
        Color.Transparent,
        MaterialTheme.colorScheme.primary.copy(alpha = 0.25f),
        MaterialTheme.colorScheme.tertiary,
        MaterialTheme.colorScheme.primary.copy(alpha = 0.25f),
        Color.Transparent,
    )

    LaunchedEffect(isRunning) {
        if (!isRunning) {
            rotationAnim.snapTo(0f)
        } else {
            isStartingConnection = false
        }
    }

    LaunchedEffect(connectionIsActive) {
        if (connectionIsActive) {
            while (true) {
                borderRotationAnim.animateTo(
                    targetValue = 360f,
                    animationSpec = tween(durationMillis = 1600, easing = LinearEasing),
                )
                borderRotationAnim.snapTo(0f)
            }
        } else {
            borderRotationAnim.snapTo(0f)
        }
    }

    Box(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .background(MaterialTheme.colorScheme.surface)
                .windowInsetsPadding(WindowInsets.navigationBars)
        ) {
            AppDivider()
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 76.dp)
                    .clickable(enabled = isRunning) { onAction(MainAction.TestCurrentServer) }
                    .padding(horizontal = 16.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column(Modifier.fillMaxWidth(0.78f)) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(
                            text = if (connectionSource == ConnectionSource.SUBSCRIPTION && connectionGroupLabel.isNotBlank()) connectionGroupLabel
                            else selectedServerLabel.orEmpty(),
                            modifier = Modifier.weight(1f),
                            style = MaterialTheme.typography.bodyMedium,
                            maxLines = 1,
                        )
                        Surface(
                            shape = MaterialTheme.shapes.small,
                            color = if (connectionSource == ConnectionSource.SUBSCRIPTION) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.tertiaryContainer,
                        ) {
                            Text(
                                stringResource(if (connectionSource == ConnectionSource.SUBSCRIPTION) R.string.fn22_auto_tag else R.string.fn22_manual_tag),
                                Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                style = MaterialTheme.typography.labelSmall,
                                color = if (connectionSource == ConnectionSource.SUBSCRIPTION) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onTertiaryContainer,
                            )
                        }
                    }
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        if (connectionIsActive) {
                            CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                        }
                        Text(
                            text = mainBottomStatusText(
                                status = if (connectionIsActive) MainStatus.Testing else status,
                                displayText = if (connectionIsActive) {
                                    stringResource(R.string.connection_test_testing)
                                } else displayText,
                                connectionTestLabel = connectionTestLabel
                                    ?: if (selectedServerDelayMillis > 0L) {
                                        stringResource(R.string.fn21_bottom_status, displayText, selectedServerDelayMillis)
                                    } else null,
                            ),
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
        }
        FloatingActionButton(
            onClick = {
                if (!isRunning) {
                    isStartingConnection = true
                    scope.launch {
                        rotationAnim.animateTo(
                            targetValue = 720f,
                            animationSpec = tween(durationMillis = 3000)
                        )
                        isStartingConnection = false
                    }
                }
                onAction(MainAction.ToggleService)
            },
            modifier = Modifier
                .align(Alignment.TopEnd)
                .padding(end = 24.dp)
                .offset(y = (-28).dp)
                .navigationBarsPadding()
                .drawWithContent {
                    drawContent()
                    if (connectionIsActive) {
                        val inset = 1.5.dp.toPx()
                        rotate(borderRotationAnim.value, pivot = center) {
                            drawRoundRect(
                                brush = Brush.sweepGradient(edgeHighlightColors, center = center),
                                topLeft = Offset(inset, inset),
                                size = Size(size.width - inset * 2, size.height - inset * 2),
                                cornerRadius = CornerRadius(18.dp.toPx()),
                                style = Stroke(width = 3.dp.toPx()),
                            )
                        }
                    }
                },
            containerColor = connectionButtonColor,
        ) {
            Icon(
                painter = if (isRunning) painterResource(R.drawable.ic_stop_24dp)
                else painterResource(R.drawable.ic_play_24dp),
                contentDescription = stringResource(
                    if (isRunning) R.string.acc_stop else R.string.acc_start
                ),
                tint = connectionButtonContentColor,
                modifier = Modifier
                    .size(24.dp)
                    .graphicsLayer { rotationZ = rotationAnim.value }
            )
        }
    }
}
