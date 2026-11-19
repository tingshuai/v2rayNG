package com.v2ray.ang.ui.main

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.Scaffold
import androidx.compose.material3.ScaffoldDefaults
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import com.v2ray.ang.R
import com.v2ray.ang.AppConfig
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.v2ray.ang.dto.entities.ProfileItem
import com.v2ray.ang.ui.compose.LocalDarkTheme
import com.v2ray.ang.ui.compose.QRCodeDialog
import kotlinx.coroutines.launch

@Composable
fun MainScreen(
    mainViewModel: MainViewModel,
    onAction: (MainAction) -> Unit,
    onNavigate: (MainDestination) -> Unit,
) {
    val uiState by mainViewModel.uiState.collectAsStateWithLifecycle()
    val groups = uiState.groups
    val isLoading by mainViewModel.isLoading.collectAsStateWithLifecycle()
    val isRunning = uiState.isRunning
    val displayText = mainViewModel.formatStatus(uiState.status)
    val selectedGuid = uiState.selectedGuid
    val confirmRemove = uiState.confirmRemove
    val shareQRCodeBitmap = uiState.shareQRCodeBitmap

    val isDarkTheme = LocalDarkTheme.current
    val drawerState = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    var showSearch by remember { mutableStateOf(false) }
    var searchQuery by remember { mutableStateOf("") }
    BackHandler(enabled = !uiState.showingSubscriptions) {
        searchQuery = ""
        showSearch = false
        onAction(MainAction.Search(""))
        onAction(MainAction.CloseSubscription)
    }
    var showDelAllConfirm by remember { mutableStateOf(false) }
    var showDelDuplicateConfirm by remember { mutableStateOf(false) }
    var showDelInvalidConfirm by remember { mutableStateOf(false) }
    var showRemoveConfirm by rememberSaveable(stateSaver = ServerDeleteTarget.Saver) {
        mutableStateOf<ServerDeleteTarget?>(null)
    }

    var shareTarget by remember { mutableStateOf<Triple<String, ProfileItem, Boolean>?>(null) }
    val removeServer: (String, String) -> Unit = { guid, profileName ->
        if (confirmRemove) {
            showRemoveConfirm = ServerDeleteTarget(guid, profileName)
        } else {
            onAction(MainAction.RemoveServer(guid))
        }
    }

    MainDialogs(
        showDelAllConfirm = showDelAllConfirm,
        onDismissDelAll = { showDelAllConfirm = false },
        onConfirmDelAll = { showDelAllConfirm = false; onAction(MainAction.RemoveAllServers) },
        showDelDuplicateConfirm = showDelDuplicateConfirm,
        onDismissDelDuplicate = { showDelDuplicateConfirm = false },
        onConfirmDelDuplicate = { showDelDuplicateConfirm = false; onAction(MainAction.RemoveDuplicateServers) },
        showDelInvalidConfirm = showDelInvalidConfirm,
        onDismissDelInvalid = { showDelInvalidConfirm = false },
        onConfirmDelInvalid = { showDelInvalidConfirm = false; onAction(MainAction.RemoveInvalidServers) },
        showRemoveConfirm = showRemoveConfirm,
        onDismissRemove = { showRemoveConfirm = null },
        onConfirmRemove = { guid -> showRemoveConfirm = null; onAction(MainAction.RemoveServer(guid)) }
    )

    if (shareTarget != null) {
        val (guid, profile, more) = shareTarget!!
        ShareMethodDialog(
            guid = guid,
            profile = profile,
            more = more,
            onDismiss = { shareTarget = null },
            onAction = onAction,
            onRemove = removeServer,
        )
    }
    if (shareQRCodeBitmap != null) {
        QRCodeDialog(bitmap = shareQRCodeBitmap, onDismiss = { onAction(MainAction.DismissQRCodeDialog) })
    }

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            MainDrawerContent(
                drawerState = drawerState,
                onNavigate = { route ->
                    scope.launch { drawerState.close() }
                    onNavigate(route)
                }
            )
        }
    ) {
        Scaffold(
            contentWindowInsets = ScaffoldDefaults.contentWindowInsets,
            topBar = {
                MainTopBar(
                    title = if (uiState.showingSubscriptions) stringResource(R.string.fn21_subscriptions)
                    else if (uiState.selectedGroupId == AppConfig.DEFAULT_SUBSCRIPTION_ID) stringResource(R.string.fn21_manual_import)
                    else if (uiState.selectedGroupId == AppConfig.FREE_NODE_SUBSCRIPTION_ID) stringResource(R.string.free_node_subscription_name)
                    else groups.firstOrNull { it.id == uiState.selectedGroupId }?.remarks.orEmpty(),
                    showBack = !uiState.showingSubscriptions,
                    onBackClick = {
                        searchQuery = ""
                        showSearch = false
                        onAction(MainAction.Search(""))
                        onAction(MainAction.CloseSubscription)
                    },
                    isLoading = isLoading,
                    showSearch = showSearch,
                    searchQuery = searchQuery,
                    onSearchQueryChange = { query: String ->
                        searchQuery = query
                        onAction(MainAction.Search(query))
                    },
                    onSearchClose = {
                        searchQuery = ""
                        onAction(MainAction.Search(""))
                        showSearch = false
                    },
                    onSearchToggle = { show: Boolean -> showSearch = show },
                    onMenuClick = { scope.launch { drawerState.open() } },
                    onAction = onAction,
                    onMoreMenuAction = { action ->
                        when (action) {
                            MainMoreMenuAction.RestartService -> onAction(MainAction.RestartService)
                            MainMoreMenuAction.DeleteAll -> showDelAllConfirm = true
                            MainMoreMenuAction.DeleteDuplicate -> showDelDuplicateConfirm = true
                            MainMoreMenuAction.DeleteInvalid -> showDelInvalidConfirm = true
                            MainMoreMenuAction.ExportAll -> onAction(MainAction.ExportAll)
                            MainMoreMenuAction.LocateSelected -> onAction(MainAction.LocateSelectedServer)
                            MainMoreMenuAction.SortByTestResults -> onAction(MainAction.SortByTestResults)
                            MainMoreMenuAction.TestAll -> onAction(MainAction.TestAllServers)
                            MainMoreMenuAction.TestAllRealPing -> onAction(MainAction.TestRealAllServers)
                            MainMoreMenuAction.UpdateSubscriptions -> onAction(MainAction.UpdateSubscriptions)
                        }
                    }
                )
            },
            bottomBar = {
                MainBottomBar(
                    displayText = displayText,
                    selectedServerLabel = uiState.selectedServerLabel,
                    selectedServerDelayMillis = uiState.selectedServerDelayMillis,
                    isRunning = isRunning,
                    isDarkTheme = isDarkTheme,
                    onAction = onAction
                )
            },
            floatingActionButton = {},
        ) { innerPadding ->
            if (uiState.showingSubscriptions) {
                SubscriptionListContent(
                    groups = groups,
                    mainViewModel = mainViewModel,
                    selectedGroupId = uiState.selectedSubscriptionId,
                    onSelect = { onAction(MainAction.SelectGroup(it)) },
                    onOpen = { searchQuery = ""; showSearch = false; onAction(MainAction.Search("")); onAction(MainAction.OpenSubscription(it)) },
                    onManage = { onNavigate(MainDestination.Subscriptions) },
                    onUpdate = { onAction(MainAction.UpdateGroupSubscription(it)) },
                    modifier = Modifier.fillMaxSize().padding(innerPadding).consumeWindowInsets(innerPadding),
                )
            } else if (groups.isNotEmpty()) {
                NodeCardsContent(
                    groupId = uiState.selectedGroupId,
                    mainViewModel = mainViewModel,
                    selectedGuid = selectedGuid,
                    locateTarget = uiState.locateTarget,
                    onLocateHandled = { onAction(MainAction.LocateHandled) },
                    usageInfo = groups.firstOrNull { it.id == uiState.selectedGroupId }?.usageInfo,
                    groupTitle = groups.firstOrNull { it.id == uiState.selectedGroupId }?.remarks.orEmpty(),
                    onOpenLink = { onAction(MainAction.OpenSubscriptionLink(it)) },
                    onRefresh = { onAction(MainAction.UpdateGroupSubscription(uiState.selectedGroupId)) },
                    selectedCountryCode = uiState.selectedCountryCode,
                    onFilterCountry = { onAction(MainAction.FilterCountry(it)) },
                    onSelect = { onAction(MainAction.SelectServer(it)) },
                    onEdit = { guid, profile -> onAction(MainAction.EditServer(guid, profile)) },
                    onShare = { guid, profile -> shareTarget = Triple(guid, profile, false) },
                    onRemove = removeServer,
                    modifier = Modifier.fillMaxSize().padding(innerPadding).consumeWindowInsets(innerPadding),
                )
            }
        }
    }
}
