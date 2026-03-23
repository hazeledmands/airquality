export function getParam(key) {
    return new URLSearchParams(location.search).get(key);
}

export function setParam(key, value) {
    const url = new URL(location.href);
    if (value == null) {
        url.searchParams.delete(key);
    } else {
        url.searchParams.set(key, value);
    }
    history.replaceState(null, '', url);
}
