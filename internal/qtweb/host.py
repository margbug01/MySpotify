# Private stdio host for MySpotify. Remote pages have no access to this protocol.
import argparse
import json
import os
from pathlib import Path
import sys
from urllib.parse import urlsplit

from PyQt6.QtCore import QByteArray, QBuffer, QCoreApplication, QDateTime, QEvent, QIODevice, QSocketNotifier, QTimer, Qt, QUrl, QT_VERSION_STR
from PyQt6.QtNetwork import QNetworkCookie, QNetworkProxy
from PyQt6.QtWidgets import QApplication
from PyQt6.QtWebEngineCore import QWebEnginePage, QWebEngineProfile, QWebEngineScript, QWebEngineSettings, QWebEngineUrlRequestInfo, QWebEngineUrlRequestInterceptor
from PyQt6.QtWebEngineWidgets import QWebEngineView

# Import WebEngine before QApplication, but do not allocate an unused shared GL context.
options = argparse.ArgumentParser()
options.add_argument('--profile', required=True)
profile_dir = Path(options.parse_args().profile)
profile_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
app = QApplication(sys.argv[:1])
app.setQuitOnLastWindowClosed(False)
proxy_url = urlsplit(os.environ.get('https_proxy') or os.environ.get('HTTPS_PROXY') or '')
if proxy_url.hostname and proxy_url.scheme in ['http', 'socks5', 'socks5h']:
    proxy_type = QNetworkProxy.ProxyType.HttpProxy if proxy_url.scheme == 'http' else QNetworkProxy.ProxyType.Socks5Proxy
    proxy = QNetworkProxy(proxy_type, proxy_url.hostname, proxy_url.port or 1080)
    if proxy_url.username:
        from urllib.parse import unquote
        proxy.setUser(unquote(proxy_url.username))
        proxy.setPassword(unquote(proxy_url.password or ''))
    QNetworkProxy.setApplicationProxy(proxy)
profile = QWebEngineProfile('MySpotify')
profile.setPersistentStoragePath(str(profile_dir))
profile.setCachePath(str(profile_dir / 'cache'))
profile.settings().setAttribute(QWebEngineSettings.WebAttribute.PlaybackRequiresUserGesture, False)


class CaptureNetwork(QWebEngineUrlRequestInterceptor):
    def interceptRequest(self, info):
        # Identity capture and audio do not need an offline web-player worker.
        if info.resourceType() == QWebEngineUrlRequestInfo.ResourceType.ResourceTypeServiceWorker:
            info.block(True)


network_filter = CaptureNetwork(profile)
profile.setUrlRequestInterceptor(network_filter)
audio_pages = {}
auth = None
auth_view = None
auth_id = None
cookies = {}
imports = {}


def reply(identifier, value=None, error=None):
    result = {'id': identifier, 'value': value}
    if error:
        result['error'] = error
    try:
        sys.stdout.write(json.dumps(result, separators=(',', ':')) + '\n')
        sys.stdout.flush()
    except BrokenPipeError:
        app.quit()


def cookie_key(cookie):
    return (bytes(cookie.name()), cookie.domain(), cookie.path())


def added(cookie):
    key = cookie_key(cookie)
    # The signal argument can refer to a temporary Qt value; retain an owned copy.
    cookies[key] = QNetworkCookie(cookie)
    for identifier, state in list(imports.items()):
        if key in state and state[key] == bytes(cookie.value()):
            del state[key]
        if not state:
            del imports[identifier]
            reply(identifier, True)


def removed(cookie):
    cookies.pop(cookie_key(cookie), None)


profile.cookieStore().cookieAdded.connect(added)
profile.cookieStore().cookieRemoved.connect(removed)
profile.cookieStore().loadAllCookies()


class Page(QWebEnginePage):
    # CaptureJS tokens, cookies and remote console messages never go to stdout/stderr.
    def javaScriptConsoleMessage(self, *args):
        pass


# Qt creates its browser context lazily. Initialize it before a cookie-only RPC;
# otherwise imports made before the first page may remain queued indefinitely.
bootstrap_page = Page(profile)
bootstrap_page.load(QUrl('about:blank'))

def bootstrap_finished(ok):
    global bootstrap_page
    bootstrap_page.deleteLater()
    bootstrap_page = None
    profile.cookieStore().loadAllCookies()

bootstrap_page.loadFinished.connect(bootstrap_finished)


class LoginView(QWebEngineView):
    def closeEvent(self, event):
        global auth, auth_view, auth_id
        page = self.page()
        if auth_view is self:
            auth = auth_view = auth_id = None
        page.deleteLater()
        super().closeEvent(event)
        self.deleteLater()


def close_auth():
    global auth, auth_view, auth_id
    page, view = auth, auth_view
    auth = auth_view = auth_id = None
    if view is not None:
        view.close()
    elif page is not None:
        page.setLifecycleState(QWebEnginePage.LifecycleState.Discarded)
        page.deleteLater()
    # Stdio callbacks can otherwise leave a closed page queued for deletion.
    QCoreApplication.sendPostedEvents(None, QEvent.Type.DeferredDelete)


def dispatch(request):
    global auth, auth_view, auth_id
    identifier = request['id']
    method = request['method']
    args = request.get('args') or {}
    if method == 'ping':
        reply(identifier, {'qt': QT_VERSION_STR})
    elif method == 'audio.open':
        target = QUrl(args['url'])
        if target.scheme() != 'http' or target.host() != '127.0.0.1':
            reply(identifier, error='音频页地址无效')
            return
        key = args['id']
        old = audio_pages.pop(key, None)
        if old is not None:
            old.deleteLater()
        page = Page(profile)
        audio_pages[key] = page
        page.load(target)
        reply(identifier, True)
    elif method == 'audio.close':
        page = audio_pages.pop(args['id'], None)
        if page is not None:
            page.deleteLater()
        reply(identifier, True)
    elif method == 'auth.open':
        target = QUrl(args['url'])
        if args['url'] != 'about:blank' and (target.scheme() != 'https' or target.host() not in ['open.spotify.com', 'accounts.spotify.com']):
            reply(identifier, error='登录页地址无效')
            return
        close_auth()
        auth_id = args['id']
        auth = Page(profile)
        preload = QWebEngineScript()
        preload.setName('MySpotify identity capture')
        preload.setInjectionPoint(QWebEngineScript.InjectionPoint.DocumentCreation)
        preload.setWorldId(QWebEngineScript.ScriptWorldId.MainWorld.value)
        preload.setRunsOnSubFrames(False)
        # Remove a previously installed worker and prevent a new one from keeping
        # the full Spotify app alive after its short-lived capture page closes.
        worker_cleanup = "try { const sw=navigator.serviceWorker; if(sw) { sw.getRegistrations().then(rs=>Promise.all(rs.map(r=>r.unregister()))).catch(()=>{}); sw.register=()=>Promise.reject(new DOMException('Offline workers disabled','NotSupportedError')); } } catch(e) {};"
        preload.setSourceCode(worker_cleanup + '\n' + args['preload'])
        auth.scripts().insert(preload)
        if args.get('show'):
            auth_view = LoginView()
            auth_view.setPage(auth)
            auth_view.setWindowTitle('登录 Spotify')
            auth_view.resize(480, 720)
            auth_view.show()
            auth_view.raise_()
            auth_view.activateWindow()
        auth.load(target)
        reply(identifier, True)
    elif method in ['auth.eval', 'auth.close', 'auth.snapshot']:
        if args['id'] != auth_id or auth is None:
            reply(identifier, error='登录窗口已关闭')
        elif method == 'auth.close':
            close_auth()
            reply(identifier, True)
        elif method == 'auth.snapshot':
            if auth_view is None:
                reply(identifier, error='登录窗口未显示')
            else:
                data = QByteArray()
                buffer = QBuffer(data)
                buffer.open(QIODevice.OpenModeFlag.WriteOnly)
                auth_view.grab().save(buffer, 'PNG')
                reply(identifier, bytes(data.toBase64()).decode('ascii'))
        else:
            auth.runJavaScript(args['script'], lambda value: reply(identifier, value))
    elif method == 'cookies.metadata':
        reply(identifier, [{'name': bytes(c.name()).decode('latin1'), 'domain': c.domain(), 'path': c.path(), 'secure': c.isSecure(), 'httpOnly': c.isHttpOnly(), 'sameSite': c.sameSitePolicy().name} for c in cookies.values()])
    elif method == 'cookies':
        if not cookies:
            profile.cookieStore().loadAllCookies()
        url = urlsplit(args['url'])
        host, path = url.hostname or '', url.path or '/'
        pairs = []
        now = QDateTime.currentDateTimeUtc()
        for cookie in cookies.values():
            domain = cookie.domain()
            matches = host == domain or (domain.startswith('.') and (host == domain[1:] or host.endswith(domain)))
            cookie_path = cookie.path() or '/'
            path_matches = path == cookie_path or (path.startswith(cookie_path) and (cookie_path.endswith('/') or path[len(cookie_path):].startswith('/')))
            expiry = cookie.expirationDate()
            if matches and path_matches and (not cookie.isSecure() or url.scheme == 'https') and (not expiry.isValid() or expiry > now):
                pairs.append(bytes(cookie.name()).decode('latin1') + '=' + bytes(cookie.value()).decode('latin1'))
        reply(identifier, '; '.join(pairs))
    elif method == 'cookies.import':
        pending = {}
        values = []
        for item in args['cookies']:
            cookie = QNetworkCookie(item['name'].encode('latin1'), item['value'].encode('latin1'))
            domain = item['domain']
            # An explicit Domain attribute would widen a host-only cookie's scope.
            cookie.setDomain(domain if domain.startswith('.') else '')
            cookie.setPath(item.get('path') or '/')
            cookie.setSecure(item.get('secure', False))
            cookie.setHttpOnly(item.get('httpOnly', False))
            same_site = {'lax': QNetworkCookie.SameSite.Lax, 'strict': QNetworkCookie.SameSite.Strict, 'none': QNetworkCookie.SameSite.None_}.get(item.get('sameSite'))
            if same_site is not None:
                cookie.setSameSitePolicy(same_site)
            if item.get('expires', 0) > 0:
                cookie.setExpirationDate(QDateTime.fromSecsSinceEpoch(item['expires']))
                if cookie.expirationDate() <= QDateTime.currentDateTimeUtc():
                    continue
            pending[(bytes(cookie.name()), domain, cookie.path())] = bytes(cookie.value())
            values.append((cookie, domain))
        if not pending:
            reply(identifier, True)
            return
        imports[identifier] = pending
        for cookie, domain in values:
            profile.cookieStore().setCookie(cookie, QUrl('https://' + domain.lstrip('.') + '/'))
        def expired():
            if identifier in imports:
                del imports[identifier]
                reply(identifier, error='登录 Cookie 导入未完成')
        QTimer.singleShot(5000, expired)
    elif method == 'cookies.clear':
        close_auth()
        profile.cookieStore().deleteAllCookies()
        deadline = QDateTime.currentMSecsSinceEpoch() + 5000
        def cleared():
            if not cookies:
                reply(identifier, True)
            elif QDateTime.currentMSecsSinceEpoch() >= deadline:
                reply(identifier, error='登录 Cookie 清理未完成')
            else:
                QTimer.singleShot(100, cleared)
        QTimer.singleShot(250, cleared)
    else:
        reply(identifier, error='未知 WebEngine 操作')


incoming = bytearray()
os.set_blocking(0, False)
notifier = QSocketNotifier(0, QSocketNotifier.Type.Read)


def read_input(*unused):
    try:
        chunk = os.read(0, 65536)
    except BlockingIOError:
        return
    if not chunk:
        notifier.setEnabled(False)
        app.quit()
        return
    incoming.extend(chunk)
    if len(incoming) > 2 * 1024 * 1024:
        app.quit()
        return
    while b'\n' in incoming:
        line, _, remainder = incoming.partition(b'\n')
        incoming[:] = remainder
        request = None
        try:
            request = json.loads(line)
            dispatch(request)
        except Exception:
            # Exceptions can contain credential values; return only a fixed error.
            if isinstance(request, dict) and isinstance(request.get('id'), int):
                reply(request['id'], error='WebEngine 操作失败')


notifier.activated.connect(read_input)
app.exec()
close_auth()
for page in audio_pages.values():
    page.deleteLater()
audio_pages.clear()
QCoreApplication.sendPostedEvents(None, QEvent.Type.DeferredDelete)
profile.deleteLater()
QCoreApplication.sendPostedEvents(None, QEvent.Type.DeferredDelete)
