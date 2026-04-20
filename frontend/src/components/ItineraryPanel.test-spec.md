# Test specification: `ItineraryPanel.tsx`

## Functions in this file

| # | Name | Kind | Role |
|---|------|------|------|
| 1 | **`ItineraryCard`** | Function component | Renders a single itinerary row: index badge (or grip in overlay mode), POI image, name, subcategory, rating, “Added by”, optional remove button when not overlay. Styles differ for `overlay`, `isDragging`, and default. |
| 2 | **`SortableItem`** | Function component | Wraps one item with `@dnd-kit` `useSortable`: drag handle button, passes `isDragging` into `ItineraryCard`, wires `onRemove`. |
| 3 | **`DeleteDayConfirm`** | Function component | Alert-dialog style confirmation: day number, stop count (singular/plural copy), **Delete Day** and **Cancel** actions. |
| 4 | **`ItineraryPanel`** | Exported function component | Main panel: local `items` state synced from `itinerary` when not dragging; groups by `day`; DnD sensors and handlers; empty state; per-day sections with delete-day flow; `DragOverlay` with `ItineraryCard` in overlay mode. |
| 5 | **`handleDragStart`** | Method inside `ItineraryPanel` | Sets `activeId` from `DragStartEvent.active.id` for overlay / drag state. |
| 6 | **`handleDragOver`** | Method inside `ItineraryPanel` | Updates `items` during drag: cross-day updates `day` on the active item when over a different day; reorders with `arrayMove` when indices differ; no-op when `over` missing, same id, or invalid indices. |
| 7 | **`handleDragEnd`** | Method inside `ItineraryPanel` | Clears `activeId`; if `over` is null, resets `items` to prop `itinerary`; otherwise calls `onReorder(items)` with the current local list. |

**Note:** A **`useEffect`** syncs `items` from the `itinerary` prop when `activeId` is null (so parent updates apply when not mid-drag). Grouping by day uses `reduce` + sorted day keys. These are not separate named functions but appear in the test table where relevant.

---

## Test table

For **component-level** rows, “inputs” are **props, state, and user/simulated actions**; “expected output” is **DOM, callbacks, or local state behavior** when the test passes.

| # | Purpose | Function / area | Test inputs | Expected output if the test passes |
|---|---------|-----------------|-------------|-----------------------------------|
| 1 | Empty itinerary | `ItineraryPanel` | `itinerary=[]`, stubs for callbacks | Renders empty state: messaging such as “No places yet” and hint about Explore tab; no day sections / lists. |
| 2 | Renders days and stops | `ItineraryPanel` | Non-empty `itinerary` with one or more `day` values | Sections per day with `aria-label` like `Day {n}`; stop counts in header; lists labeled e.g. “Stops for Day {n}”. |
| 3 | POI content on card | `ItineraryCard` / `SortableItem` | Items with `poi.name`, `poi.subcategory`, `poi.rating`, `poi.imageUrl`, `addedBy` | Visible text for name, subcategory, rating (formatted), “Added by …”; image with `alt` referencing POI name. |
| 4 | Stop index in list | `ItineraryCard` | Items in order within a day | Non-overlay cards show 1-based index per position in that day’s list (first stop = 1, …). |
| 5 | Remove button | `ItineraryCard` | Item with known `id`; click remove | `onRemove` called with that item’s `id`; button has accessible name tied to POI name. |
| 6 | Remove hidden in overlay | `ItineraryCard` | Render card with `overlay` | No remove control (overlay drag preview). |
| 7 | Rating accessibility | `ItineraryCard` | Any item with rating | Rating container exposes `aria-label` including rating value (e.g. “Rating …”). |
| 8 | Drag handle present | `SortableItem` | Rendered item | Grip control with `aria-label` including POI name (“Drag to reorder …”). |
| 9 | Toggle delete-day confirmation | `ItineraryPanel` | Click “Delete Day” control for a day | `aria-expanded` reflects open state; confirmation UI appears for that day; second click toggles closed (per `setConfirmDay` logic). |
| 10 | Delete day confirm copy | `DeleteDayConfirm` | `day`, `count` (1 and ≠1) | Heading/question references day; body uses “stop” vs “stops”; alertdialog labeling includes day. |
| 11 | Confirm delete day | `DeleteDayConfirm` → panel | Confirmation open; click **Delete Day** | `onDeleteDay` called with that `day` number; confirmation dismissed (`confirmDay` cleared). |
| 12 | Cancel delete day | `DeleteDayConfirm` | Confirmation open; click **Cancel** | `onDeleteDay` not called; confirmation dismissed. |
| 13 | Prop sync when not dragging | `useEffect` in `ItineraryPanel` | `activeId` null; `itinerary` prop changes | Local rendered list matches new `itinerary` (e.g. updated order or contents). |
| 14 | Prop sync suppressed during drag | `useEffect` in `ItineraryPanel` | While `activeId` set (simulated drag start) | Changing `itinerary` does not overwrite local `items` until drag ends / `activeId` cleared (per implementation). |
| 15 | `handleDragStart` sets active | `handleDragStart` | Programmatic or test harness `DragStartEvent` with `active.id` | `activeId` becomes that id; overlay can show active item (if asserting via DOM or instance). |
| 16 | `handleDragEnd` commits reorder | `handleDragEnd` | Drag ends with valid `over` | `onReorder` called with final `ItineraryItem[]` matching local order after drag; `activeId` cleared. |
| 17 | `handleDragEnd` cancels without drop target | `handleDragEnd` | `over` is null / undefined | `items` reset to prop `itinerary`; `onReorder` not called (per code path). |
| 18 | `handleDragOver` within-day reorder | `handleDragOver` | Two items same `day`; drag one over the other | Order of those items in `items` updates (e.g. `arrayMove` behavior). |
| 19 | `handleDragOver` cross-day move | `handleDragOver` | Active and over items different `day` | Active item’s `day` becomes over item’s `day`, and position updates relative to `over`. |
| 20 | Multiple days sorted | `ItineraryPanel` grouping | Days out of order in data (e.g. 2 then 1) | UI renders day sections in numeric order (`sort` on day keys). |

---

This specification can be mapped to **component/integration tests** (React Testing Library, `@dnd-kit` test utilities or simulated events) and, where helpful, **focused tests** around extracted pure logic if any is refactored out later.
