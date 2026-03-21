import { describe, it, expect } from 'vitest';
import { createDragHandle, createChartCard } from '../chart-card.js';

// chart-card.js uses document.createElement, which requires a DOM environment.
// DOM behavior is verified visually via Playwright screenshots.
// Here we verify the module exports exist and are functions.

describe('chart-card exports', () => {
    it('exports createDragHandle as a function', () => {
        expect(typeof createDragHandle).toBe('function');
    });

    it('exports createChartCard as a function', () => {
        expect(typeof createChartCard).toBe('function');
    });
});
