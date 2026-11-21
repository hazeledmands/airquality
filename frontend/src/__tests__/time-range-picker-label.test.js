// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from 'vitest';
import { setupTimeRangePicker } from '../time-range-picker.js';

beforeEach(() => {
    history.replaceState(null, '', '/');
    document.body.innerHTML = `
        <button id="trp-prev"></button><button id="trp-trigger"></button><button id="trp-next"></button>
        <div id="trp-popover"><div id="trp-quick"></div>
        <input id="trpFrom"><input id="trpTo"><button id="trpApply"></button></div>`;
});

// Crossing into or out of the phone layout re-renders the range label.
describe('refreshLabel', () => {
    it('switches between the short phone label and the full label', () => {
        const prev = [globalThis.innerWidth, globalThis.innerHeight];
        try {
            globalThis.innerWidth = 1280; globalThis.innerHeight = 800;
            const picker = setupTimeRangePicker({ onApply: () => {} });
            const trigger = document.getElementById('trp-trigger');
            expect(trigger.textContent).toBe('Last 3 days');
            globalThis.innerWidth = 390;
            picker.refreshLabel();
            expect(trigger.textContent).toBe('3 days');
            globalThis.innerWidth = 1280;
            picker.refreshLabel();
            expect(trigger.textContent).toBe('Last 3 days');
        } finally {
            [globalThis.innerWidth, globalThis.innerHeight] = prev;
        }
    });
});
