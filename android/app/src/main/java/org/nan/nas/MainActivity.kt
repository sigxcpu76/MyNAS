package org.nan.nas

import android.annotation.SuppressLint
import android.content.Intent
import android.graphics.Bitmap
import android.net.http.SslError
import android.os.Bundle
import android.view.View
import android.webkit.SslErrorHandler
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.OnBackPressedCallback
import androidx.appcompat.app.AppCompatActivity
import org.nan.nas.databinding.ActivityMainBinding

class MainActivity : AppCompatActivity() {
    private lateinit var binding: ActivityMainBinding
    private lateinit var prefsManager: PreferencesManager
    private var profile: ServerProfile? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        prefsManager = PreferencesManager(this)

        @Suppress("DEPRECATION")
        profile = intent.getSerializableExtra("server_profile") as? ServerProfile
            ?: prefsManager.getProfile()

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
            packageManager.getPackageInfo(packageName, 0).versionName ?: "1.6.10"
        } catch (e: Exception) {
            "1.6.10"
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
        }

        binding.webView.webViewClient = object : WebViewClient() {
            override fun onReceivedSslError(view: WebView?, handler: SslErrorHandler?, error: SslError?) {
                if (profile?.trustSelfSigned == true) {
                    // Bypass self-signed SSL errors when explicitly configured
                    handler?.proceed()
                } else {
                    handler?.cancel()
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
                val targetHost = profile?.host ?: ""
                // Keep server URLs inside the WebView; allow external links to open outside
                if (targetHost.isNotEmpty() && uri.host?.contains(targetHost) == true) {
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
        }
    }

    private fun injectTokenAndPreferences() {
        val token = profile?.token ?: ""
        val ver = getAppVersionName()
        val build = getAppVersionCode()

        // Inject stored auth token, APK version metadata, and force mobile-specific layout
        val js = """
            (function() {
                try {
                    window.nanNativeApp = {
                        platform: 'android',
                        version: '$ver',
                        build: $build
                    };
                    if (typeof window !== 'undefined' && window.localStorage) {
                        window.localStorage.setItem('nan_apk_version', '$ver');
                        window.localStorage.setItem('nan_apk_build', '$build');
                        if ('$token'.length > 0) {
                            if (!window.localStorage.getItem('nas_jwt_token')) {
                                window.localStorage.setItem('nas_jwt_token', '$token');
                            }
                        }
                        window.localStorage.setItem('nas_device_view_mode', 'mobile');
                        window.localStorage.setItem('device_ui_mode_override', 'mobile');
                    }
                } catch(e) {}
            })();
        """.trimIndent()

        binding.webView.evaluateJavascript(js, null)
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
