export const DEFAULT_ORDER = ['temp', 'dewpoint', 'humidity', 'pm', 'acState'];

/**
 * Parse an order URL param into a valid id array.
 * Unknown ids are dropped; missing ids are appended in their default position.
 */
export function parseOrder(param, validIds = DEFAULT_ORDER) {
    if (!param) return [...validIds];
    const seen = new Set();
    const order = [];
    for (const id of param.split(',')) {
        if (validIds.includes(id) && !seen.has(id)) {
            seen.add(id);
            order.push(id);
        }
    }
    for (const id of validIds) {
        if (!seen.has(id)) order.push(id);
    }
    return order;
}

/** Serialize order to a comma-separated string, or null if it matches the default. */
export function serializeOrder(order, defaultOrder = DEFAULT_ORDER) {
    if (order.length === defaultOrder.length && order.every((id, i) => id === defaultOrder[i])) return null;
    return order.join(',');
}

/** Reorder DOM children of container to match the given order using data-chart-id. */
export function applyOrder(container, order) {
    for (const id of order) {
        const el = container.querySelector(`[data-chart-id="${id}"]`);
        if (el) container.appendChild(el);
    }
}

/**
 * Initialize drag-to-reorder on a container.
 *
 * Options:
 *   itemSelector  — CSS selector for draggable children (default: '.chart-card')
 *   dataAttr      — data attribute name storing the item id (default: 'chartId')
 *   urlParam      — URL search param name to persist order (default: 'order')
 *   defaultOrder  — array of ids in their default order (used to decide whether
 *                   to write the URL param). If null, order is always persisted.
 */
export function setupReorder(container, {
    itemSelector = '.chart-card',
    dataAttr = 'chartId',
    urlParam = 'order',
    defaultOrder = null,
} = {}) {
    let dragEl = null;
    let touchStartY = 0;
    let touchClone = null;

    function getItems() {
        return Array.from(container.querySelectorAll(itemSelector));
    }

    function getCurrentOrder() {
        return getItems().map(el => el.dataset[dataAttr]);
    }

    function persistOrder() {
        const order = getCurrentOrder();
        const val = defaultOrder ? serializeOrder(order, defaultOrder) : order.join(',');
        const url = new URL(window.location);
        if (val) {
            url.searchParams.set(urlParam, val);
        } else {
            url.searchParams.delete(urlParam);
        }
        history.replaceState(null, '', url);
    }

    // --- Mouse / pointer drag-and-drop ---

    function onDragStart(e) {
        const item = e.target.closest(itemSelector);
        if (!item) return;
        dragEl = item;
        item.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', item.dataset[dataAttr]);
    }

    function onDragOver(e) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        const target = e.target.closest(itemSelector);
        if (!target || target === dragEl) return;

        const rect = target.getBoundingClientRect();
        const midY = rect.top + rect.height / 2;
        if (e.clientY < midY) {
            container.insertBefore(dragEl, target);
        } else {
            container.insertBefore(dragEl, target.nextSibling);
        }
    }

    function onDragEnd() {
        if (dragEl) {
            dragEl.classList.remove('dragging');
            dragEl = null;
        }
        persistOrder();
    }

    // --- Touch drag-and-drop (mobile) ---

    function onTouchStart(e) {
        const handle = e.target.closest('.drag-handle');
        if (!handle) return;
        const item = handle.closest(itemSelector);
        if (!item) return;

        dragEl = item;
        touchStartY = e.touches[0].clientY;
        const rect = item.getBoundingClientRect();

        touchClone = item.cloneNode(true);
        touchClone.classList.add('drag-clone');
        touchClone.style.position = 'fixed';
        touchClone.style.left = rect.left + 'px';
        touchClone.style.top = rect.top + 'px';
        touchClone.style.width = rect.width + 'px';
        touchClone.style.zIndex = '9999';
        touchClone.style.opacity = '0.85';
        touchClone.style.pointerEvents = 'none';
        document.body.appendChild(touchClone);

        item.classList.add('dragging');
    }

    function onTouchMove(e) {
        if (!dragEl || !touchClone) return;
        e.preventDefault();

        const touch = e.touches[0];
        const deltaY = touch.clientY - touchStartY;
        const origRect = dragEl.getBoundingClientRect();
        touchClone.style.top = (origRect.top + deltaY) + 'px';

        const items = getItems().filter(el => el !== dragEl);
        for (const item of items) {
            const rect = item.getBoundingClientRect();
            if (touch.clientY >= rect.top && touch.clientY <= rect.bottom) {
                const midY = rect.top + rect.height / 2;
                if (touch.clientY < midY) {
                    container.insertBefore(dragEl, item);
                } else {
                    container.insertBefore(dragEl, item.nextSibling);
                }
                break;
            }
        }
    }

    function onTouchEnd() {
        if (touchClone) {
            touchClone.remove();
            touchClone = null;
        }
        if (dragEl) {
            dragEl.classList.remove('dragging');
            dragEl = null;
        }
        persistOrder();
    }

    /** Attach drag behavior to current items. Call again after DOM rebuild. */
    function bindItems() {
        for (const item of getItems()) {
            // Track where mousedown occurred so dragstart can check
            // (e.target in dragstart is the draggable element, not the clicked child)
            let startedFromHandle = false;
            item.addEventListener('mousedown', (e) => {
                startedFromHandle = !!e.target.closest('.drag-handle');
                item.setAttribute('draggable', startedFromHandle ? 'true' : 'false');
            });
            item.addEventListener('dragstart', (e) => {
                if (!startedFromHandle) {
                    e.preventDefault();
                    return;
                }
                onDragStart(e);
            });
        }
    }

    container.addEventListener('dragover', onDragOver);
    container.addEventListener('dragend', onDragEnd);
    container.addEventListener('touchstart', onTouchStart, { passive: true });
    container.addEventListener('touchmove', onTouchMove, { passive: false });
    container.addEventListener('touchend', onTouchEnd);

    bindItems();

    // Restore order from URL
    const saved = new URLSearchParams(window.location.search).get(urlParam);
    if (saved) {
        const validIds = defaultOrder || getCurrentOrder();
        applyOrder(container, parseOrder(saved, validIds));
    }

    return { bindItems };
}
