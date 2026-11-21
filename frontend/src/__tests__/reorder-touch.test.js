// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setupReorder } from '../reorder.js';

function build() {
    document.body.innerHTML = `<div id="c">
        <div class="room-section" data-chart-id="a"><span class="drag-handle">≡</span><div style="height:100px"></div></div>
        <div class="room-section" data-chart-id="b"><span class="drag-handle">≡</span><div style="height:100px"></div></div>
    </div>`;
    const c = document.getElementById('c');
    // jsdom has no layout: give each section a fake box.
    [...c.children].forEach((el, i) => {
        el.getBoundingClientRect = () => ({ top: i * 100, bottom: i * 100 + 100, left: 0, width: 300, height: 100 });
    });
    return c;
}

function touch(target, type, clientY) {
    const ev = new Event(type, { bubbles: true, cancelable: true });
    ev.touches = type === 'touchend' ? [] : [{ clientY }];
    target.dispatchEvent(ev);
    return ev;
}

beforeEach(() => history.replaceState(null, '', '/'));

describe('touch reorder', () => {
    it('a tap on a handle (no drag) does not write ?roomOrder', () => {
        const c = build();
        setupReorder(c, { itemSelector: '.room-section', urlParam: 'roomOrder' });
        const spy = vi.spyOn(history, 'replaceState');
        const h = c.querySelector('[data-chart-id="a"] .drag-handle');
        touch(h, 'touchstart', 10);
        touch(h, 'touchend');
        expect(spy).not.toHaveBeenCalled();
        expect(location.search).toBe('');
        spy.mockRestore();
    });

    it('a real drag reorders and persists', () => {
        const c = build();
        setupReorder(c, { itemSelector: '.room-section', urlParam: 'roomOrder' });
        const h = c.querySelector('[data-chart-id="a"] .drag-handle');
        touch(h, 'touchstart', 10);
        touch(h, 'touchmove', 180);
        touch(h, 'touchend');
        expect(new URLSearchParams(location.search).get('roomOrder')).toBe('b,a');
    });

    // A non-passive touchmove on the whole container makes every scroll wait
    // for JavaScript; it is only listened to while a handle drag runs.
    it('only listens to touchmove (non-passively) during a handle drag', () => {
        const c = build();
        const add = vi.spyOn(c, 'addEventListener');
        setupReorder(c, { itemSelector: '.room-section', urlParam: 'roomOrder' });
        const moves = () => add.mock.calls.filter(([type]) => type === 'touchmove');
        expect(moves()).toHaveLength(0);
        const h = c.querySelector('[data-chart-id="a"] .drag-handle');
        touch(h, 'touchstart', 10);
        expect(moves().length).toBeGreaterThan(0);
        const rm = vi.spyOn(c, 'removeEventListener');
        touch(h, 'touchend');
        expect(rm.mock.calls.some(([type]) => type === 'touchmove')).toBe(true);
    });

    it('a touchmove that does not start on a handle is never cancelled', () => {
        const c = build();
        setupReorder(c, { itemSelector: '.room-section', urlParam: 'roomOrder' });
        const body = c.querySelector('[data-chart-id="a"] div');
        touch(body, 'touchstart', 10);
        const ev = touch(body, 'touchmove', 60);
        expect(ev.defaultPrevented).toBe(false);
    });
});
