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
                if (binding.webView.canGoBack()) {
                    binding.webView.goBack()
                } else {
                    finish()
                }
            }
        })
    }

    private fun setupToolbar() {
        binding.tvToolbarSubtitle.text = "${profile?.host}:${profile?.port}"

        binding.btnReload.setOnClickListener {
            binding.webView.reload()
        }

        binding.btnSwitchServer.setOnClickListener {
            redirectToLogin()
        }

        binding.swipeRefresh.setColorSchemeColors(getColor(R.color.accent_primary))
        binding.swipeRefresh.setOnRefreshListener {
            binding.webView.reload()
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun setupWebView() {
        val webSettings = binding.webView.settings
        webSettings.javaScriptEnabled = true
        webSettings.domStorageEnabled = true
        webSettings.databaseEnabled = true
        webSettings.useWideViewPort = true
        webSettings.loadWithOverviewMode = true
        webSettings.cacheMode = WebSettings.LOAD_DEFAULT
        webSettings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        webSettings.mediaPlaybackRequiresUserGesture = false

        // Custom User-Agent to allow web application to identify Android native shell
        val defaultUA = webSettings.userAgentString
        webSettings.userAgentString = "$defaultUA NaN-NAS-Android/1.5.0"

        binding.webView.webChromeClient = object : WebChromeClient() {
            override fun onProgressChanged(view: WebView?, newProgress: Int) {
                if (newProgress < 100) {
                    binding.pageProgressBar.visibility = View.VISIBLE
                    binding.pageProgressBar.progress = newProgress
                } else {
                    binding.pageProgressBar.visibility = View.GONE
                    binding.swipeRefresh.isRefreshing = false
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
                binding.swipeRefresh.isRefreshing = false
            }

            override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
                val url = request?.url?.toString() ?: return false
                val targetHost = profile?.host ?: ""
                // Keep all server URLs inside the WebView; allow external links to open outside if needed
                if (url.contains(targetHost)) {
                    return false
                }
                return false
            }
        }
    }

    private fun injectTokenAndPreferences() {
        val token = profile?.token ?: ""
        if (token.isEmpty()) return

        // Inject stored auth token and force mobile-specific layout
        val js = """
            (function() {
                try {
                    if (typeof window !== 'undefined' && window.localStorage) {
                        if (!window.localStorage.getItem('nas_jwt_token') || window.localStorage.getItem('nas_jwt_token') !== '$token') {
                            window.localStorage.setItem('nas_jwt_token', '$token');
                        }
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
