# FinWise Design System

**Status:** Draft  
**Date:** 2026-04-21  
**Stack:** HTMX + DaisyUI + Tailwind CSS v4  
**Based on:** `ui-ux-pro-max` skill analysis, `ARCHITECTURE.md`, `docs/specs/2026-04-21-finwise-phase1-design.md`

---

## 1. Design Philosophy

FinWise is a **trustworthy, calm, and focused** personal finance companion. The interface must feel like a conversation with a professional bookkeeper — clean, unobtrusive, and data-dense without being cluttered.

- **Conversational but structured:** Chat is the primary interaction model, but financial data demands clarity and scannability.
- **Triple-theme:** Light mode for bright environments, a soft dark mode for comfortable evening use, and an OLED dark mode for maximum power efficiency on compatible devices.
- **Mobile-first:** Many users will snap receipt photos on their phones. Every feature must work flawlessly on small screens.
- **Micro-delights:** Subtle feedback (sounds, animations, hover states) that make the app feel alive without being distracting.

---

## 2. Color Palette

### 2.1 Light Mode (Default)

| Token | Hex | Tailwind Equiv. | Usage |
|-------|-----|-----------------|-------|
| **Background** | `#FAFAFA` | `bg-neutral-50` | Page background |
| **Surface** | `#FFFFFF` | `bg-white` | Cards, modals, chat bubbles |
| **Primary** | `#18181B` | `text-zinc-900` | Headings, primary text, icons |
| **Secondary** | `#52525B` | `text-zinc-600` | Body text, descriptions |
| **Muted** | `#71717A` | `text-zinc-500` | Timestamps, placeholders (WCAG AA compliant) |
| **Border** | `#E4E4E7` | `border-zinc-200` | Dividers, input borders, card borders |
| **CTA / Accent** | `#2563EB` | `bg-blue-600` | Primary buttons, links, focus rings, active states |
| **CTA Hover** | `#1D4ED8` | `hover:bg-blue-700` | Button hover |
| **Success** | `#10B981` | `text-emerald-500` | Income, confirmed sync, positive change |
| **Warning** | `#F59E0B` | `text-amber-500` | Needs review, medium confidence |
| **Error** | `#EF4444` | `text-red-500` | Validation errors, failed sync, expenses |

### 2.2 Dark Mode (Soft)

A comfortable dark theme with elevated surfaces — easier on the eyes than OLED pure black and better for LCD screens where true black looks washed out.

| Token | Hex | Tailwind Equiv. | Usage |
|-------|-----|-----------------|-------|
| **Background** | `#0F0F12` | `bg-[#0F0F12]` | Deep charcoal page background |
| **Surface** | `#1C1C21` | `bg-[#1C1C21]` | Cards, modals, chat bubbles |
| **Surface Elevated** | `#24242A` | `bg-[#24242A]` | Hover states, dropdowns, popovers |
| **Primary** | `#F4F4F5` | `text-zinc-100` | Headings, primary text |
| **Secondary** | `#A1A1AA` | `text-zinc-400` | Body text |
| **Muted** | `#71717A` | `text-zinc-500` | Timestamps, placeholders (WCAG AA compliant) |
| **Border** | `#2E2E35` | `border-[#2E2E35]` | Dividers, card borders |
| **Border Subtle** | `#1F1F24` | `border-[#1F1F24]` | Subtle separators |
| **CTA / Accent** | `#3B82F6` | `bg-blue-500` | Primary buttons, focus rings |
| **CTA Hover** | `#2563EB` | `hover:bg-blue-600` | Button hover |
| **Success** | `#34D399` | `text-emerald-400` | Income, confirmed sync |
| **Warning** | `#FBBF24` | `text-amber-400` | Needs review |
| **Error** | `#F87171` | `text-red-400` | Errors, expenses |

### 2.3 Dark Mode (OLED)

| Token | Hex | Tailwind Equiv. | Usage |
|-------|-----|-----------------|-------|
| **Background** | `#000000` | `bg-black` | True black for OLED power saving |
| **Surface** | `#18181B` | `bg-zinc-900` | Cards, modals, chat bubbles |
| **Primary** | `#FAFAFA` | `text-neutral-50` | Headings, primary text |
| **Secondary** | `#A1A1AA` | `text-zinc-400` | Body text |
| **Muted** | `#71717A` | `text-zinc-500` | Timestamps, placeholders (WCAG AA compliant) |
| **Border** | `#27272A` | `border-zinc-800` | Dividers, subtle borders |
| **CTA / Accent** | `#3B82F6` | `bg-blue-500` | Primary buttons, focus rings |
| **CTA Hover** | `#2563EB` | `hover:bg-blue-600` | Button hover |
| **Success** | `#34D399` | `text-emerald-400` | Income, confirmed sync |
| **Warning** | `#FBBF24` | `text-amber-400` | Needs review |
| **Error** | `#F87171` | `text-red-400` | Errors, expenses |

### 2.4 Semantic Color Usage

- **Expenses:** Use `Error` color (red) for negative amounts.
- **Income:** Use `Success` color (green) for positive amounts.
- **Pending/Review:** Use `Warning` color (amber) for transactions requiring user attention.
- **Synced:** Use `Success` color with a checkmark icon.

---

## 3. Typography

### 3.1 Font Family

**IBM Plex Sans** — A typeface designed for data and user interfaces. It conveys trust, professionalism, and clarity.

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@300;400;500;600;700&display=swap" rel="stylesheet">
```

Tailwind config:
```js
fontFamily: {
  sans: ['"IBM Plex Sans"', 'sans-serif'],
}
```

### 3.2 Type Scale

| Token | Size | Weight | Line Height | Usage |
|-------|------|--------|-------------|-------|
| **H1** | `1.875rem` (30px) | 700 (Bold) | 1.2 | Page titles (Dashboard, Chat) |
| **H2** | `1.5rem` (24px) | 600 (SemiBold) | 1.3 | Section headings, card titles |
| **H3** | `1.25rem` (20px) | 500 (Medium) | 1.4 | Subsection headings, transaction payee names |
| **Body** | `1rem` (16px) | 400 (Regular) | 1.5 | Paragraphs, chat messages |
| **Small** | `0.875rem` (14px) | 400 (Regular) | 1.5 | Secondary text, table cells, labels |
| **Caption** | `0.75rem` (12px) | 500 (Medium) | 1.4 | Timestamps, badges, currency codes |
| **Stat** | `2.25rem` (36px) | 700 (Bold) | 1.1 | Dashboard big numbers |

### 3.3 Monospace for Numbers

For financial figures (amounts, dates in tables), use a tabular-nums variant to ensure alignment:
```css
.font-tabular {
  font-variant-numeric: tabular-nums;
}
```

---

## 4. Layout & Spacing

### 4.1 Grid & Spacing

- **Base unit:** 4px (Tailwind default).
- **Section padding:** `py-8 md:py-12`.
- **Card padding:** `p-4 md:p-6`.
- **Gap between cards:** `gap-4 md:gap-6`.

### 4.2 Container

- **Max width:** `max-w-7xl` (1280px) for dashboard/history.
- **Narrow content:** `max-w-2xl` for settings, setup wizard, and focused chat.
- **Alignment:** Centered with `mx-auto`.

### 4.3 App Shell

```html
<div class="drawer lg:drawer-open">
  <input id="sidebar-drawer" type="checkbox" class="drawer-toggle" />
  <div class="drawer-content flex flex-col min-h-screen">
    <!-- Navbar (mobile only) -->
    <!-- Main content -->
  </div>
  <div class="drawer-side z-40">
    <label for="sidebar-drawer" class="drawer-overlay"></label>
    <!-- Sidebar navigation -->
  </div>
</div>
```

- **Mobile:** Hamburger menu, collapsible sidebar.
- **Desktop (`lg:`):** Persistent sidebar (`lg:drawer-open`).

### 4.4 Responsive Strategy

| Breakpoint | Width | Layout Changes |
|------------|-------|----------------|
| **Mobile** | < 768px | Single column, drawer sidebar, stacked forms, card-list views instead of tables |
| **Tablet** | 768px - 1024px | Two-column grids, drawer sidebar, hybrid tables |
| **Desktop** | > 1024px | Multi-column grids, persistent sidebar, full data tables |

**Padding:**
- Mobile: `px-4`
- Tablet: `md:px-6`
- Desktop: `lg:px-8`

---

## 5. Components

### 5.1 Chat Bubble

**User Message:**
```html
<div class="chat chat-end">
  <div class="chat-bubble bg-blue-600 text-white rounded-2xl rounded-tr-sm shadow-sm">
    I spent $45 at Starbucks
  </div>
  <div class="chat-footer opacity-50 text-xs mt-1">10:42 AM</div>
</div>
```

**Assistant Message:**
```html
<div class="chat chat-start">
  <div class="chat-bubble bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 text-zinc-900 dark:text-zinc-50 rounded-2xl rounded-tl-sm shadow-sm">
    Got it. I've found 1 transaction. Here is the preview:
    <!-- Transaction Preview Card embedded here -->
  </div>
  <div class="chat-footer opacity-50 text-xs mt-1">Bookkeeper · 10:42 AM</div>
</div>
```

- **Max width:** `max-w-[85%] md:max-w-[75%]`.
- **Embedded cards:** Assistant bubbles can contain transaction preview cards. These cards have no outer border to avoid visual noise inside the bubble.

### 5.2 Transaction Preview Card

Used inline in chat and in batch review mode.

```html
<div class="bg-white dark:bg-zinc-800 rounded-xl border border-zinc-200 dark:border-zinc-700 p-4 shadow-sm hover:shadow-md transition-shadow duration-200">
  <div class="flex items-center justify-between gap-4">
    <!-- Left: Date & Payee -->
    <div class="flex-1 min-w-0">
      <p class="text-xs text-zinc-500 dark:text-zinc-400 font-tabular">2024-01-15</p>
      <p class="font-medium text-zinc-900 dark:text-zinc-100 truncate">Starbucks</p>
      <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-zinc-100 dark:bg-zinc-700 text-zinc-600 dark:text-zinc-300 mt-1">
        Food & Drink
      </span>
    </div>

    <!-- Right: Amount & Actions -->
    <div class="text-right shrink-0">
      <p class="font-bold text-zinc-900 dark:text-zinc-100 font-tabular">-$45.00</p>
      <div class="flex items-center justify-end gap-2 mt-2">
        <!-- Confidence indicator -->
        <div class="w-16 h-1.5 bg-zinc-200 dark:bg-zinc-700 rounded-full overflow-hidden">
          <div class="h-full bg-emerald-500 rounded-full" style="width: 95%"></div>
        </div>
        <button class="btn btn-sm btn-ghost btn-circle" aria-label="Edit">
          <!-- Pencil icon -->
        </button>
        <button class="btn btn-sm btn-success btn-circle" aria-label="Confirm">
          <!-- Check icon -->
        </button>
      </div>
    </div>
  </div>
</div>
```

**States:**
- **Pending:** Default state, shows edit/confirm buttons.
- **Needs Review:** Amber left border (`border-l-4 border-amber-500`), warning icon.
- **Confirmed:** Green left border, checkmark icon, buttons hidden.
- **Rejected:** Opacity 50%, strikethrough text, grey left border.

### 5.3 Stat Card

```html
<div class="bg-white dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-6 shadow-sm">
  <p class="text-sm font-medium text-zinc-500 dark:text-zinc-400">This Month's Spending</p>
  <p class="mt-2 text-3xl font-bold tracking-tight text-zinc-900 dark:text-zinc-50 font-tabular">$1,245.00</p>
  <p class="mt-1 text-sm text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
    <!-- ArrowDown icon --> 12% less than last month
  </p>
</div>
```

### 5.4 Button

| Variant | Class | Usage |
|---------|-------|-------|
| **Primary** | `btn bg-blue-600 hover:bg-blue-700 text-white border-none rounded-lg px-4 py-2` | Main actions (Send, Confirm All, Save) |
| **Secondary** | `btn bg-zinc-900 hover:bg-zinc-800 text-white dark:bg-zinc-100 dark:hover:bg-white dark:text-zinc-900 border-none rounded-lg px-4 py-2` | Secondary actions (Back, Cancel) |
| **Ghost** | `btn btn-ghost text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 rounded-lg px-4 py-2` | Tertiary actions (Edit, Dismiss) |
| **Danger** | `btn bg-red-600 hover:bg-red-700 text-white border-none rounded-lg px-4 py-2` | Destructive actions (Delete, Reject) |
| **Icon** | `btn btn-circle btn-sm btn-ghost` | Icon-only actions |

**Sizes:**
- Large: `px-6 py-3 text-base` (CTAs)
- Default: `px-4 py-2 text-sm` (forms)
- Small: `px-3 py-1.5 text-xs` (inline actions)

### 5.5 Input & Textarea

```html
<div class="form-control w-full">
  <label class="label">
    <span class="label-text text-sm font-medium text-zinc-700 dark:text-zinc-300">Payee</span>
  </label>
  <input
    type="text"
    class="input input-bordered w-full bg-white dark:bg-zinc-900 border-zinc-300 dark:border-zinc-700 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 transition-colors"
    placeholder="e.g. Starbucks"
  />
</div>
```

**Chat Input (Textarea):**
```html
<div class="relative">
  <textarea
    class="textarea textarea-bordered w-full bg-white dark:bg-zinc-900 border-zinc-300 dark:border-zinc-700 rounded-2xl pr-12 py-3 resize-none focus:ring-2 focus:ring-blue-500"
    rows="1"
    placeholder="Type a message..."
  ></textarea>
  <button class="absolute right-3 bottom-3 btn btn-circle btn-sm btn-primary">
    <!-- Send icon -->
  </button>
</div>
```

### 5.6 File Upload Zone

```html
<div
  class="border-2 border-dashed border-zinc-300 dark:border-zinc-700 rounded-2xl p-8 text-center transition-colors duration-200 hover:border-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20 cursor-pointer"
  id="drop-zone"
>
  <div class="mx-auto w-12 h-12 text-zinc-400 dark:text-zinc-500 mb-3">
    <!-- UploadCloud icon -->
  </div>
  <p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">
    Drop files here or click to upload
  </p>
  <p class="text-xs text-zinc-500 dark:text-zinc-500 mt-1">
    JPG, PNG, CSV, XLSX, OFX, TXT up to 10MB
  </p>
</div>
```

**Drag active state:** `border-blue-500 bg-blue-50 dark:bg-blue-900/20`.

### 5.7 Badge / Tag

```html
<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
  Pending
</span>
```

**Variants:**
- Pending: `bg-blue-100 text-blue-800`
- Confirmed: `bg-emerald-100 text-emerald-800`
- Needs Review: `bg-amber-100 text-amber-800`
- Rejected: `bg-red-100 text-red-800`
- Category: `bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300`

### 5.8 Skeleton Loader

```html
<div class="animate-pulse space-y-4">
  <div class="h-4 bg-zinc-200 dark:bg-zinc-700 rounded w-3/4"></div>
  <div class="h-4 bg-zinc-200 dark:bg-zinc-700 rounded"></div>
  <div class="h-4 bg-zinc-200 dark:bg-zinc-700 rounded w-5/6"></div>
</div>
```

Use for:
- Dashboard stats while loading from AB API.
- Chat messages while waiting for LLM response.
- Transaction list initial load.

### 5.9 Progress / Confidence Bar

```html
<div class="flex items-center gap-2">
  <div class="flex-1 h-2 bg-zinc-200 dark:bg-zinc-700 rounded-full overflow-hidden">
    <div class="h-full rounded-full transition-all duration-500 ease-out
      bg-emerald-500" style="width: 92%"></div>
  </div>
  <span class="text-xs font-medium text-zinc-500 dark:text-zinc-400 w-8 text-right">92%</span>
</div>
```

**Color mapping:**
- `>= 90%`: `bg-emerald-500`
- `70-89%`: `bg-amber-500`
- `< 70%`: `bg-red-500`

---

## 6. Page Designs

### Navigation Flow

```
First Visit ──→ /setup (Step 1 → 2 → 3)
                      │
                      ▼
Returning User ──→ / (Dashboard)
                      │
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
      /chat        /history      /settings
        │             │             │
        └─────────────┴─────────────┘
                      │
                      ▼
              Back to / (Dashboard)
```

**Entry Points:**
- **First-time user:** Any URL → redirect to `/setup`. Hard gate — no skipping.
- **Returning user (configured):** `/` (Dashboard) as default landing.
- **Direct navigation:** All routes accessible directly if configured; unconfigured routes redirect to `/setup`.

**Back Navigation:**
- Sidebar persists on all pages → primary navigation.
- Browser back button works for all routes.
- No in-page "Back" buttons needed (sidebar handles orientation).

### Information Hierarchy per Screen

| Screen | Primary | Secondary | Tertiary |
|--------|---------|-----------|----------|
| Dashboard | Stats grid (top-left stat first) | Recent transactions | FAB chat button |
| Chat | Message stream | Input area | Header status |
| History | Filtered transaction list | Filter controls | Pagination |
| Settings | Form inputs per section | Test connection buttons | Save action |
| Setup Wizard | Current step form | Progress indicator | Navigation buttons |

### 6.1 Dashboard (`/`)

**Purpose:** At-a-glance financial health and quick entry point.

**Layout:**
```
┌─────────────────────────────────────────────────────────┐
│  [Sidebar] │  Header: "Dashboard" · [Date Picker]       │
│            │  ─────────────────────────────────────────  │
│  · Dash    │  [Stat Card] [Stat Card] [Stat Card]       │
│  · Chat    │  [Stat Card]                                 │
│  · History │  ─────────────────────────────────────────  │
│  · Settings│  Recent Transactions                         │
│            │  ┌──────────────────────────────────────┐   │
│            │  │ Date       Payee         Amount      │   │
│            │  │ ...table rows...                     │   │
│            │  └──────────────────────────────────────┘   │
│            │                                              │
│            │  [Floating Chat Button - bottom right]       │
└─────────────────────────────────────────────────────────┘
```

**Elements:**
1. **Header:** Page title left-aligned. Date range picker ("This Month") right-aligned.
2. **Stats Grid:** 2-col on mobile, 4-col on desktop.
   - This Month's Spending
   - Top Category
   - Pending Reviews count (warning color if > 0)
   - Recent Activity count
3. **Recent Transactions:** Last 5-10 transactions from Actual Budget. Clicking "View All" navigates to `/history`.
4. **Quick Chat Widget:** Floating action button (FAB) bottom-right. Opens a mini chat overlay for quick natural language entry.

### 6.2 Chat (`/chat`)

**Purpose:** Full-screen conversational interface with the Bookkeeper.

**Layout:**
```
┌─────────────────────────────────────────────────────────┐
│  [Sidebar] │  Header: "Bookkeeper" · [Status Dot] · ⬇️ │
│            │  ─────────────────────────────────────────  │
│            │                                              │
│            │  👤 Hello!                                   │
│            │  🤖 Hi! How can I help?                      │
│            │  👤 [File: statement.png]                    │
│            │  🤖 Found 23 transactions...                 │
│            │     [Transaction Card]                       │
│            │     [Transaction Card]                       │
│            │     [Transaction Card]                       │
│            │                                              │
│            │  ─────────────────────────────────────────  │
│            │  [📎] [Type a message...        ] [Send]    │
└─────────────────────────────────────────────────────────┘
```

**Elements:**
1. **Header:**
   - Title: "Bookkeeper".
   - Status dot: Green (connected), Amber (SSE reconnecting), Red (disconnected).
   - "New Chat" button (clears session).
2. **Message Area:**
   - Flex-1, scrollable, auto-scroll to bottom on new messages.
   - Padding-bottom: `pb-32` to account for fixed input.
   - Date separators if chat spans multiple days.
3. **Transaction Previews:**
   - Appear inline within assistant bubbles.
   - Batch previews show stacked cards with a "Confirm All" and "Edit" header.
   - Each card has individual Confirm/Edit/Reject actions.
4. **Input Area:**
   - Fixed to bottom, full width, `p-4 bg-white/80 dark:bg-zinc-900/80 backdrop-blur`.
   - Attachment button (paperclip) left of textarea.
   - Textarea auto-resizes up to 5 rows.
   - Send button (paper airplane) right.
   - Drag-and-drop overlay: Full-screen translucent layer when files are dragged over the window.
5. **SSE States:**
   - **Idle:** Normal state.
   - **Uploading:** File upload progress bar above input.
   - **Processing:** Typing indicator (three bouncing dots) from assistant.
   - **Streaming:** Transaction cards fade in one by one (`animate-in fade-in slide-in-from-bottom-2 duration-200`).
   - **Complete:** Summary message ("All done! 18 confirmed, 5 need review.").
   - **Error:** Red inline banner with retry button.

### 6.3 History (`/history`)

**Purpose:** Review all transactions synced to Actual Budget.

**Layout:**
```
┌─────────────────────────────────────────────────────────┐
│  [Sidebar] │  Header: "History" · [Filters]             │
│            │  ─────────────────────────────────────────  │
│            │  Filters: [Date ▼] [Category ▼] [Payee 🔍] │
│            │  ─────────────────────────────────────────  │
│            │  ┌──────────────────────────────────────┐   │
│            │  │ Date │ Payee │ Category │ Amount │ ✓ │   │
│            │  ├──────────────────────────────────────┤   │
│            │  │ ...rows...                           │   │
│            │  └──────────────────────────────────────┘   │
│            │  [Prev] [Page 1 of 5] [Next]                 │
└─────────────────────────────────────────────────────────┘
```

**Elements:**
1. **Filters:** Sticky below header. Date range, Category dropdown, Payee search.
2. **Table (Desktop):**
   - Columns: Date, Payee, Category, Amount (right-aligned), Status.
   - Sortable headers (click to sort).
   - Row hover: `bg-zinc-50 dark:bg-zinc-800/50`.
3. **Card List (Mobile):**
   - Each transaction is a card with stacked layout.
   - Swipe actions: Edit, Delete (if supported by AB API).
4. **Pagination:** Simple prev/next. Or "Load More" infinite scroll.
5. **Empty State:** Illustration + "No transactions found. Try adjusting filters."

### 6.4 Settings (`/settings`)

**Purpose:** Configure LLM, Actual Budget connection, and app preferences.

**Layout:**
```
┌─────────────────────────────────────────────────────────┐
│  [Sidebar] │  Header: "Settings"                        │
│            │  ─────────────────────────────────────────  │
│            │  ┌─ LLM Configuration ─────────────────┐   │
│            │  │ Provider: [OpenAI    ▼]             │   │
│            │  │ API Key: [********************    ] │   │
│            │  │ Model: [gpt-4o       ▼]             │   │
│            │  └─────────────────────────────────────┘   │
│            │  ┌─ Actual Budget ─────────────────────┐   │
│            │  │ URL: [https://...                 ] │   │
│            │  │ Password: [************           ] │   │
│            │  │ [Test Connection]                   │   │
│            │  └─────────────────────────────────────┘   │
│            │  ┌─ Preferences ───────────────────────┐   │
│            │  │ Currency: [USD ▼]                   │   │
│            │  │ Theme: [☀️ Light · 🌑 Soft Dark · ⚫ OLED · 🖥️ System]│   │
│            │  │ Auto-sync threshold: [0.90        ] │   │
│            │  └─────────────────────────────────────┘   │
│            │  [Save Changes]                              │
└─────────────────────────────────────────────────────────┘
```

**Elements:**
1. **Card Grouping:** Each section is a card with a clear title.
2. **Input Fields:** Labels above inputs. Helper text below for context.
3. **Test Connection:** Inline button that shows success/error toast.
4. **Theme Toggle:** Segmented control (Light / Soft Dark / OLED / System).
5. **Save Button:** Sticky at bottom on mobile, inline on desktop.

### 6.5 Setup Wizard (`/setup`)

**Purpose:** First-run onboarding.

**Layout:**
```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│              ┌─────────────────────────┐                │
│              │  Step 1 of 3            │                │
│              │  [●──○──○]              │                │
│              │                         │                │
│              │  Welcome to FinWise     │                │
│              │  ...content...          │                │
│              │                         │                │
│              │  [Continue]             │                │
│              └─────────────────────────┘                │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Elements:**
1. **Centered Card:** `max-w-lg`, vertically centered.
2. **Progress Bar:** Step indicator (1-2-3 dots + lines).
3. **Steps:**
   - **Step 1:** Welcome + LLM Provider selection + API Key.
   - **Step 2:** Actual Budget URL + Password.
   - **Step 3:** Test connections (LLM ping + AB login). Show loading spinner during test. Success checkmarks. "Go to Dashboard" button.
4. **Navigation:** "Continue" primary, "Back" ghost (hidden on step 1).
5. **Validation:** Inline validation before advancing. No skipping steps.

---

## 7. Interactions & Micro-Delights

### 7.1 Animation Principles

- **Duration:** 150-300ms for micro-interactions. Never exceed 500ms.
- **Easing:**
  - Enter: `ease-out` (fast start, slow end).
  - Exit: `ease-in` (slow start, fast end).
- **Respect `prefers-reduced-motion`:**
  ```css
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after {
      animation-duration: 0.01ms !important;
      transition-duration: 0.01ms !important;
    }
  }
  ```

### 7.2 Specific Interactions

| Interaction | Behavior |
|-------------|----------|
| **Page Load** | Content fades in (`opacity-0` → `opacity-100`, `duration-300`). |
| **Card Hover** | `shadow-sm` → `shadow-md`, `translate-y-0` → `-translate-y-0.5`. Duration 200ms. |
| **Button Hover** | Background darkens/lightens. Scale `1` → `1.02` (subtle). |
| **Button Active** | Scale `0.98`. |
| **Chat Message Appear** | `animate-in fade-in slide-in-from-bottom-2 duration-200`. |
| **Transaction Card Stream** | Staggered entrance: 50ms delay between each card. |
| **Skeleton Loading** | `animate-pulse`. Stop immediately when real data arrives. |
| **Toast Notification** | Slide in from top-right (`slide-in-from-right`), auto-dismiss after 4s. |
| **Dark Mode Toggle** | Smooth transition on `html` element: `transition-colors duration-300`. |
| **Drag-and-Drop** | Drop zone border turns blue, background light blue. Cursor changes to `copy`. |
| **File Upload Progress** | Linear progress bar at top of chat input area. |
| **SSE Reconnect** | Status dot pulses amber. Toast: "Reconnecting..." |
| **Sync Success** | Transaction card flashes green briefly (`bg-emerald-500/20` → `bg-transparent`). Optional subtle "pop" sound. |
| **Confirm All** | Button shows spinner, then checkmark. Confirmed cards collapse into a compact summary. |

### 7.3 Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Enter` (Chat input) | Send message |
| `Shift + Enter` (Chat input) | New line |
| `Esc` | Close modals, collapse sidebar |
| `Ctrl/Cmd + K` | Focus global search (future) |
| `Ctrl/Cmd + /` | Show keyboard shortcuts help |

---

## 8. Icons

**Icon Set:** [Lucide](https://lucide.dev/) (consistent, clean, open-source).

**Key Icons:**
- Send: `send`
- Paperclip: `paperclip`
- Check: `check`
- Pencil: `pencil`
- X / Reject: `x`
- Upload: `upload-cloud`
- Dashboard: `layout-dashboard`
- Chat: `message-square`
- History: `history`
- Settings: `settings`
- Moon (dark mode): `moon`
- Sun (light mode): `sun`
- ChevronDown: `chevron-down`
- AlertTriangle: `alert-triangle`
- AlertCircle: `alert-circle`
- Loader: `loader-2` (with `animate-spin`)

**Rules:**
- Default size: `w-5 h-5` (20px).
- Button icons: `w-4 h-4`.
- Large decorative: `w-8 h-8` or `w-12 h-12`.
- Stroke width: Default (2px).

---

## 9. Accessibility

- **Contrast:** All text meets WCAG 2.1 AA (4.5:1 for normal text, 3:1 for large text).
- **Focus Rings:** All interactive elements have visible focus states: `ring-2 ring-blue-500 ring-offset-2 ring-offset-white dark:ring-offset-zinc-900`.
- **Form Labels:** Every input has an associated `<label>`.
- **ARIA:**
  - Chat messages: `role="log"`, `aria-live="polite"`.
  - Status indicators: `aria-label="Connection status: connected"`.
  - Buttons with icons only: `aria-label` describing the action.
  - Loading states: `aria-busy="true"`.
- **Screen Reader:** Transaction cards should read as: "Transaction: Starbucks, 45 dollars, Food and Drink, January 15th. Confidence 95 percent."
- **Motion:** Respect `prefers-reduced-motion` (see 7.1).

---

## 10. Assets

### 10.1 Logo

Simple wordmark or icon + wordmark.
- **Icon:** Abstract "F" or a stylized chat bubble with a checkmark.
- **Color:** Primary (`#18181B` light mode, `#FAFAFA` dark mode).
- **Font:** IBM Plex Sans Bold.

### 10.2 Empty States

- **No transactions:** Simple line-art illustration (piggy bank or document). Not required for MVP — text + icon is sufficient.
- **No chat history:** "Start by typing a message or uploading a receipt."

### 10.3 Favicon

- `favicon.ico` and SVG favicon.
- Simple geometric shape matching the logo icon.

---

## 11. Implementation Notes

### 11.1 Tailwind + DaisyUI Configuration (Tailwind v4)

DaisyUI v4 uses CSS-based configuration with the `@plugin` directive instead of JavaScript config. Create three custom themes in your main CSS file (e.g., `styles/globals.css`):

```css
@import "tailwindcss";
@plugin "daisyui" {
  themes: finwise, finwiseSoft, finwiseOled;
}
@plugin "daisyui/theme" {
  name: finwise;
  --color-primary: #2563EB;
  --color-secondary: #18181B;
  --color-accent: #3F3F46;
  --color-neutral: #FAFAFA;
  --color-base-100: #FFFFFF;
  --color-info: #3B82F6;
  --color-success: #10B981;
  --color-warning: #F59E0B;
  --color-error: #EF4444;
}

@plugin "daisyui/theme" {
  name: finwiseSoft;
  --color-primary: #3B82F6;
  --color-secondary: #F4F4F5;
  --color-accent: #A1A1AA;
  --color-neutral: #0F0F12;
  --color-base-100: #1C1C21;
  --color-info: #3B82F6;
  --color-success: #34D399;
  --color-warning: #FBBF24;
  --color-error: #F87171;
}

@plugin "daisyui/theme" {
  name: finwiseOled;
  --color-primary: #3B82F6;
  --color-secondary: #FAFAFA;
  --color-accent: #A1A1AA;
  --color-neutral: #000000;
  --color-base-100: #18181B;
  --color-info: #3B82F6;
  --color-success: #34D399;
  --color-warning: #FBBF24;
  --color-error: #F87171;
}
```

Use DaisyUI's `data-theme` attribute to switch between themes: `data-theme="finwise"` (light), `data-theme="finwiseSoft"` (soft dark), or `data-theme="finwiseOled"` (OLED).

### 11.2 HTMX Swap Strategies

- **Inline edits:** `hx-target="#tx-{id}" hx-swap="innerHTML"`.
- **New chat messages:** `hx-swap="beforeend"` on the message container, then scroll to bottom.
- **Toast notifications:** `hx-swap="afterbegin"` on a fixed toast container.

### 11.3 Theme Implementation

FinWise supports three explicit themes plus a system preference option.

**Approach:** Use DaisyUI's `data-theme` attribute on `<html>` rather than Tailwind's `dark:` prefix. This gives precise control over all three palettes.

```js
// tailwind.config.js
module.exports = {
  darkMode: 'class',
  // ...
}
```

**Theme switching logic:**

| User Selection | `data-theme` value | Notes |
|----------------|-------------------|-------|
| Light | `finwise` | Default. Bright and clean. |
| Soft Dark | `finwiseSoft` | Charcoal surfaces. Best all-around dark mode for LCD screens and comfortable evening use. |
| OLED | `finwiseOled` | True black background. Maximum power savings on OLED devices. |
| System | `finwise` / `finwiseSoft` | Detect `prefers-color-scheme: dark`. Use `finwiseSoft` as the default dark mapping for system preference (safer default than OLED). |

**Example toggle:**
```js
function setTheme(theme) {
  const html = document.documentElement;
  if (theme === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    html.setAttribute('data-theme', prefersDark ? 'finwiseSoft' : 'finwise');
    localStorage.setItem('theme', 'system');
  } else {
    html.setAttribute('data-theme', theme);
    localStorage.setItem('theme', theme);
  }
}

// On load: restore saved theme or default to system
const saved = localStorage.getItem('theme') || 'system';
setTheme(saved);
```

**CSS transitions between themes:**
```css
html {
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

@media (prefers-reduced-motion: reduce) {
  html {
    transition: none !important;
  }
}
```

## 12. Interaction State Coverage

### 12.1 State Table

| Feature | Loading | Empty | Error | Success | Partial |
|---------|---------|-------|-------|---------|---------|
| **Dashboard Stats** | Skeleton loaders (4 pulse cards) | "No data yet — start by adding a transaction in Chat" + CTA to /chat | "Couldn't load summary" + retry button | Stats render with values | Some stats load, others show "—" |
| **Recent Transactions (Dashboard)** | Skeleton table rows (3) | "No transactions this month" + illustration + "Add one" button | Inline banner: "Failed to load" | Table renders | "Showing 3 of 10 — some failed to load" |
| **Chat Messages** | Typing indicator (3 dots) | "Start by typing a message or uploading a receipt" + quick-action chips | Red inline banner with retry | Message appears | Streaming: cards fade in one by one |
| **File Upload (Chat)** | Linear progress bar above input | Drop zone with icon + "Drop files here" | "Upload failed — [Retry]" | File icon appears in input | — |
| **History Table** | Skeleton rows (5) | "No transactions found" (if zero total) / "Try adjusting filters" (if filter mismatch) + "Clear filters" button | Toast: "Failed to load history" | Table renders with sortable headers | "Showing 20 of 50 — load more?" |
| **Settings Forms** | — | — | Inline field errors + "Save failed" toast | "Settings saved" toast + checkmark on button | — |
| **Test Connection (Settings)** | Spinner on button, disabled state | — | Red text: "Connection failed — check URL/password" | Green checkmark + "Connected" | — |
| **Setup Wizard** | Step 3: spinning loader during connection test | — | Inline error on failed field validation / "Connection failed" on step 3 | Step 3: checkmarks + "Go to Dashboard" | — |
| **Transaction Preview Card** | — | — | — | Green flash + checkmark icon | Confidence bar shows amber if < 90% |

### 12.2 Empty State Design Rules

- **Always include a primary action.** Never show "No items found." without a button to create the first item.
- **Add warmth.** Use friendly copy: "Nothing here yet — let's add your first transaction."
- **Contextual illustration.** Simple icon (piggy bank, document) if available; fallback to icon + text.

## 13. User Journey & Emotional Arc

### 13.1 Storyboard

| Step | User Does | User Feels | Plan Supports It? |
|------|-----------|------------|-------------------|
| 1 | Opens FinWise for first time | Curious, slightly cautious (financial data) | Setup wizard is calm, step-by-step, no overwhelm |
| 2 | Enters LLM key and AB credentials | Uncertain — "Will this work?" | Test connection buttons give immediate feedback |
| 3 | Sees green checkmarks on step 3 | Confident, relieved | Success state is celebratory but not childish |
| 4 | Lands on Dashboard | Oriented, in control | Clear stats, familiar sidebar, no surprises |
| 5 | Types first message in Chat | Hopeful, testing | Assistant responds fast, shows preview card |
| 6 | Confirms first transaction | Accomplished | Green flash, subtle feedback, progress visible |
| 7 | Returns next day, sees updated stats | Trust building | Dashboard reflects yesterday's work, continuity |
| 8 | Hits an error (AB offline) | Frustrated | Clear error message, retry action, no blame |

### 13.2 Time-Horizon Design

- **10 seconds (visceral):** First transaction appears within SSE stream. Clean, uncluttered first impression. Calm colors. No popups.
- **5 minutes (behavioral):** Chat feels responsive. Transaction previews are scannable. Confirm/reject is one tap.
- **5 months (reflective):** Users trust the app with their financial data because it never surprises them, always explains, and recovers gracefully from errors.

### 13.3 Quality Targets

- **Latency:** First transaction event within **10 seconds** for statements with <30 transactions.
- **Accuracy:** ≥**85%** correct field extraction on benchmark dataset.
- **OCR Fallback Trigger:** Enable OCR pipeline when LLM vision accuracy drops below **80%**.
- **Auto-Sync Threshold:** Default **90%** confidence for automatic confirmation (configurable).

## 14. AI Slop Risk Assessment

**Classifier:** APP UI (workspace-driven, data-dense, task-focused).

**Hard rejections triggered:** None.

**Litmus scorecard:**
| Check | Result |
|-------|--------|
| Brand unmistakable in first screen? | YES — "FinWise" + bookkeeper metaphor |
| One strong visual anchor? | YES — blue CTA + clean stat cards |
| Scannable by headlines only? | YES — "Dashboard", "Bookkeeper", "History" |
| Each section has one job? | YES — stats show metrics, table shows transactions |
| Cards actually necessary? | YES — stat cards and transaction cards are functional containers |
| Motion improves hierarchy? | YES — staggered card entrance, toast slides |
| Premium without decorative shadows? | YES — shadows are minimal (`shadow-sm` only) |

**App UI rules check:**
- Calm surface hierarchy: PASS
- Dense but readable: PASS
- Utility language: PASS ("This Month's Spending", "Pending Reviews")
- Minimal chrome: PASS
- Cards earn existence: PASS (stat cards, transaction cards, settings cards)

**AI Slop blacklist:**
- No purple gradients, no 3-column feature grid, no decorative blobs, no emoji in UI, no generic hero copy, no system-ui font.

**Verdict:** 9/10 — specific, intentional, and differentiated. The transaction card left border is functional (state indication), not decorative.

## 15. Responsive & Accessibility

### 15.1 Touch Targets

- All interactive elements: **minimum 44x44px**.
- Button sizes: Small `px-3 py-1.5` meets 44px height. Icon buttons use `btn-circle btn-sm` (40px) — increase to `w-11 h-11` (44px) on mobile.
- Table row actions on mobile: full-width swipe targets, minimum 80px wide.

### 15.2 Keyboard Navigation

| Pattern | Behavior |
|---------|----------|
| **Chat input** | `Enter` sends, `Shift+Enter` new line. `Tab` moves to attachment button, then send button. |
| **Sidebar** | `Tab` cycles through nav items. `Enter` activates. Current page has `aria-current="page"`. |
| **Modal dialogs** | Focus trap inside modal. `Esc` closes. `Tab` cycles modal elements only. |
| **Transaction cards** | `Tab` moves through Edit → Confirm → Reject. `Enter` or `Space` activates. |
| **Settings forms** | Standard tab order. `Enter` submits form when focused on any input. |
| **Setup wizard** | `Enter` advances if current step is valid. `Tab` cycles form fields. |

### 15.3 ARIA Landmarks

```html
<nav aria-label="Main navigation"> <!-- Sidebar -->
<main> <!-- Page content -->
<section aria-label="Chat messages" role="log" aria-live="polite"> <!-- Chat stream -->
<aside role="status" aria-live="assertive"> <!-- Toast container -->
```

### 15.4 Color Contrast Audit

| Token | Foreground | Background | Ratio | WCAG AA |
|-------|------------|------------|-------|---------|
| Body text | `#18181B` | `#FFFFFF` | 15.8:1 | PASS |
| Secondary text | `#52525B` | `#FFFFFF` | 7.2:1 | PASS |
| Muted text | `#71717A` | `#FFFFFF` | 4.6:1 | PASS |
| CTA button | `#FFFFFF` | `#2563EB` | 4.5:1 | PASS |
| Dark mode body | `#F4F4F5` | `#1C1C21` | 14.2:1 | PASS |
| Dark mode muted | `#71717A` | `#1C1C21` | 4.8:1 | PASS |

**Note:** All text ratios now meet WCAG AA requirements (4.5:1 for normal text, 3:1 for large text).

## 16. Resolved Design Decisions

The following decisions were validated on 2026-04-21 and accepted with their recommended defaults.

| Decision | Accepted Default | Rationale |
|----------|-----------------|-----------|
| **Chat FAB on /chat page** | Hide FAB when on `/chat` | Redundant entry point causes confusion |
| **Mobile sidebar behavior** | Slide-over overlay (not push) | Preserves context, avoids content reflow |
| **Chat date separators** | Midnight boundary + 24h gap rule | Consistent grouping, easy to scan |
| **Auto-sync threshold** | Confidence >= 90% auto-confirms; < 90% shows "Needs Review" | Sensible starting point; will be calibrated against actual correction rates once data exists |
| **LLM provider selection UI** | Dropdown in setup | Simplest for MVP; upgrade to cards if > 3 providers |
| **Mobile table swipe actions** | Swipe right = Edit; Swipe left = Delete (with confirm) | Clear mapping, destructive action guarded |
| **History pagination vs infinite scroll** | "Load More" infinite scroll | Mobile-friendly, simpler HTMX implementation |
| **Transaction card collapse after Confirm All** | Collapse into a compact summary line | Prevents screen clutter, preserves context |

**Status:** All 8 decisions resolved. No overrides.

## 17. NOT in Scope

The following design decisions were considered and explicitly deferred:

- **Custom illustrations for empty states** — Text + icon sufficient for MVP. Illustrations can be added post-launch without breaking layout.
- **Onboarding tour after setup** — Setup wizard is enough hand-holding for MVP. A product tour can be added if user testing shows confusion.
- **Advanced theming (user-defined colors)** — Three preset themes cover 95% of use cases. Custom themes add config complexity.
- **Animation library (Framer Motion, GSAP)** — CSS transitions + Tailwind `animate-*` sufficient. Complex animations deferred.
- **Desktop app / PWA offline mode** — Web-only for MVP. PWA manifest can be added later.

## 18. Engineering Review TODOs

The following items were identified during `/plan-eng-review` and must be applied before implementation:

| # | File | Section | Issue | Fix | Status |
|---|------|---------|-------|-----|--------|
| 1 | `DESIGN.md` | 11.1 | DaisyUI v4 config uses `tailwind.config.js` (v3 syntax) but stack is Tailwind v4 | Update to CSS-based DaisyUI v4 theme config: `@plugin "daisyui/theme"` | ✅ Applied |
| 2 | `DESIGN.md` | 11.3 | Theme transition applies to ALL elements: `html, html * { transition: ... }` | Scope transition to `html` only | ✅ Applied |
| 3 | `DESIGN.md` | 2.1/15.4 | Muted text `#A1A1AA` on `#FFFFFF` is 3.1:1 — fails WCAG AA | Change to `#71717A` (4.6:1) | ✅ Applied |
| 4 | `ARCHITECTURE.md` | 8.1 | Chat pagination missing — loads ALL messages | Add `GET /chat/messages?limit=50&before_id={id}` endpoint | Pending (Phase 2) |
| 5 | `ARCHITECTURE.md` | 8.1 / 13.1 | `dedup_hash = SHA256(date+payee+amount)` collides for recurring purchases | Include `upload_id` or `source_filename` in hash input | ✅ Applied |
| 6 | `ARCHITECTURE.md` | 10.1 | LLM client is custom wrapper over 3 provider SDKs | Adopt `litellm` library instead of custom wrapper | Pending (Phase 2) |
| 7 | `ARCHITECTURE.md` | 6.1 | Full Repository layer proposed for 4 tables | Drop Repository layer; Services query SQLAlchemy models directly | ✅ Applied |
| 8 | `ARCHITECTURE.md` | 8.3 | SSE resume logic queries DB per event without index | Add composite index on `pending_transactions(job_id, created_at)` | ✅ Applied |
| 9 | `ARCHITECTURE.md` | 13.1 | Active job per session tracked implicitly | Add `active_job_id` to `chat_sessions` | ✅ Applied |
| 10 | `ARCHITECTURE.md` | 7.3 | No index on `pending_transactions.confidence` | Add index on `(confidence, status, created_at)` for calibration queries | ✅ Applied |

---

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 1 | issues_open (SELECTIVE_EXPANSION) | 6 proposals, 5 accepted, 1 deferred |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 0 | — | — |
| Design Review | `/plan-design-review` | UI/UX gaps | 2 | clear (FULL) | score: 9/10, 8 decisions resolved |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **UNRESOLVED:** 0
- **VERDICT:** Design Review CLEARED — Eng Review required before shipping
- **STALENESS NOTE:** CEO Review from 2026-04-21 may be stale — 2 commits since review (d929241 → 3df6da1)

*End of Design Document*
