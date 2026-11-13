package org.nan.nas

import android.annotation.SuppressLint
import android.content.Intent
import android.graphics.Bitmap
import android.net.Uri
import android.net.http.SslError
import android.os.Bundle
import android.os.Message
import android.view.View
import android.webkit.SslErrorHandler
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.OnBackPressedCallback
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import org.json.JSONObject
import org.nan.nas.databinding.ActivityMainBinding

class MainActivity : AppCompatActivity() {
    private lateinit var binding: ActivityMainBinding
    private lateinit var prefsManager: PreferencesManager
    private var profile: ServerProfile? = null
    private var shouldSeedProfileToken: Boolean = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        prefsManager = PreferencesManager(this)

        @Suppress("DEPRECATION")
        val incomingProfile = intent.getSerializableExtra("server_profile") as? ServerProfile
        shouldSeedProfileToken = incomingProfile != null && !incomingProfile.token.isNullOrEmpty()
        profile = incomingProfile ?: prefsManager.getProfile()

        if (profile == null || profile?.host.isNullOrEmpty()) {
            redirectToLogin()
            return
        }

        setupToolbar()
        setupWebView()
        setupBackNavigation()
        loadServerUrl()
    }

    private fun setupBackNavigation() {
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                binding.webView.evaluateJavascript(
                    "(function() { if (typeof window.nanDismissTopOverlay === 'function') { return window.nanDismissTopOverlay(); } return false; })();"
                ) { result ->
                    val dismissed = result == "true" || result == "\"true\""
                    if (!dismissed) {
                        if (binding.webView.canGoBack()) {
                            binding.webView.goBack()
                        } else {
                            finish()
                        }
                    }
                }
            }
        })
    }

    private fun getAppVersionName(): String {
        return try {
            packageManager.getPackageInfo(packageName, 0).versionName ?: "1.6.12"
        } catch (e: Exception) {
            "1.6.12"
        }
    }

    private fun getAppVersionCode(): Long {
        return try {
            val pInfo = packageManager.getPackageInfo(packageName, 0)
            if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.P) {
                pInfo.longVersionCode
            } else {
                @Suppress("DEPRECATION")
                pInfo.versionCode.toLong()
            }
        } catch (e: Exception) {
            9L
        }
    }

    private fun setupToolbar() {
        val ver = getAppVersionName()
        val build = getAppVersionCode()
        binding.tvToolbarSubtitle.text = "${profile?.host}:${profile?.port} • APK v$ver (b$build)"

        binding.btnReload.setOnClickListener {
            binding.webView.reload()
        }

        binding.btnSwitchServer.setOnClickListener {
            redirectToLogin()
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun setupWebView() {
        binding.webView.overScrollMode = View.OVER_SCROLL_NEVER

        val webSettings = binding.webView.settings
        webSettings.javaScriptEnabled = true
        webSettings.domStorageEnabled = true
        webSettings.databaseEnabled = true
        webSettings.useWideViewPort = true
        webSettings.loadWithOverviewMode = true
        webSettings.cacheMode = WebSettings.LOAD_DEFAULT
        webSettings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        webSettings.mediaPlaybackRequiresUserGesture = false
        webSettings.setSupportMultipleWindows(true)
        webSettings.javaScriptCanOpenWindowsAutomatically = true

        val ver = getAppVersionName()
        val build = getAppVersionCode()

        // Custom User-Agent to allow web application to identify Android native shell
        val defaultUA = webSettings.userAgentString
        webSettings.userAgentString = "$defaultUA NaN-NAS-Android/$ver (Build $build)"

        binding.webView.addJavascriptInterface(object : Any() {
            @android.webkit.JavascriptInterface
            fun getVersionName(): String = ver
            @android.webkit.JavascriptInterface
            fun getVersionCode(): Long = build
            @android.webkit.JavascriptInterface
            fun isNativeApp(): Boolean = true
            @android.webkit.JavascriptInterface
            fun updateToken(newToken: String) {
                profile?.let {
                    val updated = it.copy(token = newToken)
                    profile = updated
                    prefsManager.saveProfile(updated)
                }
            }
            @android.webkit.JavascriptInterface
            fun clearSession() {
                profile?.let {
                    val updated = it.copy(token = "")
                    profile = updated
                    prefsManager.saveProfile(updated)
                }
                runOnUiThread {
                    redirectToLogin()
                }
            }
        }, "NanAndroidNative")

        binding.webView.webChromeClient = object : WebChromeClient() {
            override fun onProgressChanged(view: WebView?, newProgress: Int) {
                if (newProgress < 100) {
                    binding.pageProgressBar.visibility = View.VISIBLE
                    binding.pageProgressBar.progress = newProgress
                } else {
                    binding.pageProgressBar.visibility = View.GONE
                }
            }

            override fun onCreateWindow(
                view: WebView?,
                isDialog: Boolean,
                isUserGesture: Boolean,
                resultMsg: Message?
            ): Boolean {
                val transport = resultMsg?.obj as? WebView.WebViewTransport ?: return false
                val popupWebView = WebView(this@MainActivity)
                popupWebView.webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(
                        popupView: WebView?,
                        request: WebResourceRequest?
                    ): Boolean {
                        val uri = request?.url ?: return false
                        routePopupOrNavigationUri(uri, isPopup = true)
                        popupView?.destroy()
                        return true
                    }
                }
                transport.webView = popupWebView
                resultMsg.sendToTarget()
                return true
            }
        }

        binding.webView.webViewClient = object : WebViewClient() {
            override fun onReceivedSslError(view: WebView?, handler: SslErrorHandler?, error: SslError?) {
                if (profile?.trustSelfSigned == true) {
                    // Bypass self-signed SSL errors when explicitly configured
                    handler?.proceed()
                } else {
                    if (isFinishing || isDestroyed) {
                        handler?.cancel()
                        redirectToLogin()
                        return
                    }
                    AlertDialog.Builder(this@MainActivity)
                        .setTitle("Untrusted TLS Certificate")
                        .setMessage("The server TLS certificate is not trusted. Do you want to proceed anyway?")
                        .setPositiveButton("Proceed") { _, _ ->
                            handler?.proceed()
                        }
                        .setNegativeButton("Cancel") { _, _ ->
                            handler?.cancel()
                            redirectToLogin()
                        }
                        .setOnCancelListener {
                            handler?.cancel()
                            redirectToLogin()
                        }
                        .show()
                }
            }

            override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
                super.onPageStarted(view, url, favicon)
                injectTokenAndPreferences()
            }

            override fun onPageFinished(view: WebView?, url: String?) {
                super.onPageFinished(view, url)
                injectTokenAndPreferences()
            }

            override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
                val uri = request?.url ?: return false
                return routePopupOrNavigationUri(uri, isPopup = false)
            }
        }
    }

    private fun resolvePort(uri: Uri): Int {
        if (uri.port != -1) return uri.port
        return if (uri.scheme.equals("https", ignoreCase = true)) 443 else 80
    }

    private fun isSameServerOrigin(uri: Uri): Boolean {
        val baseUrl = profile?.baseUrl ?: return false
        val baseUri = Uri.parse(baseUrl)
        val baseHost = baseUri.host ?: return false
        val uriHost = uri.host ?: return false
        return uriHost.equals(baseHost, ignoreCase = true) &&
            (uri.port == baseUri.port || resolvePort(uri) == resolvePort(baseUri))
    }

    private fun routePopupOrNavigationUri(uri: Uri, isPopup: Boolean): Boolean {
        // Keep same-server (matching host and port) URLs inside the WebView;
        // open external links or Docker app portals on different ports via ACTION_VIEW
        if (isSameServerOrigin(uri)) {
            if (isPopup) {
                binding.webView.loadUrl(uri.toString())
                return true
            }
            return false
        }
        return try {
            val intent = Intent(Intent.ACTION_VIEW, uri)
            startActivity(intent)
            true
        } catch (e: Exception) {
            false
        }
    }

    private fun injectTokenAndPreferences() {
        val tokenQuoted = JSONObject.quote(profile?.token ?: "")
        val verQuoted = JSONObject.quote(getAppVersionName())
        val build = getAppVersionCode()
        val forceSeed = shouldSeedProfileToken
        val isTablet = resources.configuration.smallestScreenWidthDp >= 600
        val defaultViewModeQuoted = JSONObject.quote(if (isTablet) "auto" else "mobile")

        // Inject stored auth token, APK version metadata, and default device layout if unset
        val js = """
            (function() {
                try {
                    var token = $tokenQuoted;
                    var ver = $verQuoted;
                    var defaultMode = $defaultViewModeQuoted;
                    window.nanNativeApp = {
                        platform: 'android',
                        version: ver,
                        build: $build
                    };
                    if (typeof window !== 'undefined' && window.localStorage) {
                        window.localStorage.setItem('nan_apk_version', ver);
                        window.localStorage.setItem('nan_apk_build', String($build));
                        var isHttpOrigin = window.location && /^https?:$/i.test(window.location.protocol);
                        var seeded = false;
                        if (token.length > 0 && isHttpOrigin) {
                            if ($forceSeed || !window.localStorage.getItem('nas_jwt_token')) {
                                window.localStorage.setItem('nas_jwt_token', token);
                                seeded = true;
                            }
                        }
                        if (!window.localStorage.getItem('nas_device_view_mode')) {
                            window.localStorage.setItem('nas_device_view_mode', defaultMode);
                        }
                        if (!window.localStorage.getItem('device_ui_mode_override')) {
                            window.localStorage.setItem('device_ui_mode_override', defaultMode);
                        }
                        return seeded ? 'seeded' : 'ok';
                    }
                } catch(e) {}
                return 'noop';
            })();
        """.trimIndent()

        binding.webView.evaluateJavascript(js) { result ->
            if (forceSeed && (result == "\"seeded\"" || result == "seeded")) {
                shouldSeedProfileToken = false
            }
        }
    }

    private fun loadServerUrl() {
        val url = profile?.baseUrl ?: return
        injectTokenAndPreferences()
        binding.webView.loadUrl(url)
    }

    private fun redirectToLogin() {
        val intent = Intent(this, LoginActivity::class.java).apply {
            putExtra("force_relogin", true)
        }
        startActivity(intent)
        finish()
    }
}
