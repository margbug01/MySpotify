// Runs before open.spotify.com's own scripts. It only notes the access token and
// client token the web player obtains for itself, so Vinyl can ask for licences
// with the same identity; nothing on the page changes.
(() => {
  if (location.hostname !== 'open.spotify.com') return;
  const v = (window.__vinyl = {});
  const wanted = /\/api\/token|clienttoken/;
  const header = (h, name) => {
    if (!h) return null;
    if (h instanceof Headers) return h.get(name);
    if (Array.isArray(h)) {
      const e = h.find((p) => String(p[0]).toLowerCase() === name);
      return e ? e[1] : null;
    }
    for (const k in h) if (k.toLowerCase() === name) return h[k];
    return null;
  };
  const noteHeader = (name, value) => {
    if (!value) return;
    if (name === 'client-token') v.clientToken = value;
    if (name === 'authorization' && /^Bearer /i.test(value)) v.bearer = value.slice(7);
  };
  const keep = (url, text) => {
    try {
      const j = JSON.parse(text);
      if (/\/api\/token/.test(url) && j.accessToken) v.token = j;
      if (/clienttoken/.test(url) && j.granted_token) v.clientToken = j.granted_token.token;
    } catch (e) {}
  };

  const fetch0 = window.fetch;
  window.fetch = function (input, init) {
    try {
      for (const h of [init && init.headers, input instanceof Request ? input.headers : null]) {
        noteHeader('client-token', header(h, 'client-token'));
        noteHeader('authorization', header(h, 'authorization'));
      }
    } catch (e) {}
    const url = typeof input === 'string' ? input : (input && input.url) || String(input);
    const p = fetch0.apply(this, arguments);
    if (wanted.test(url)) p.then((r) => r.clone().text()).then((t) => keep(url, t)).catch(() => {});
    return p;
  };

  const open0 = XMLHttpRequest.prototype.open;
  const set0 = XMLHttpRequest.prototype.setRequestHeader;
  XMLHttpRequest.prototype.open = function (method, url) {
    const u = String(url);
    if (wanted.test(u)) {
      this.addEventListener('load', () => {
        try { keep(u, this.responseText); } catch (e) {}
      });
    }
    return open0.apply(this, arguments);
  };
  XMLHttpRequest.prototype.setRequestHeader = function (k, value) {
    noteHeader(String(k).toLowerCase(), value);
    return set0.apply(this, arguments);
  };
})();
