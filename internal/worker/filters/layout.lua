-- Second filter. Page layout decisions that are independent of the Template:
-- table column widths and heading depth.

local NATURAL_TABLE_MAX_CHARS = 60
local MIN_COLUMN_SHARE = 0.06
local CHARS_PER_LINE = 55
local MAX_HEADING_LEVEL = 5

local function cell_texts(tbl)
  local rows = {}
  for _, row in ipairs(tbl.head.rows) do rows[#rows + 1] = row end
  for _, body in ipairs(tbl.bodies) do
    for _, row in ipairs(body.head) do rows[#rows + 1] = row end
    for _, row in ipairs(body.body) do rows[#rows + 1] = row end
  end
  for _, row in ipairs(tbl.foot.rows) do rows[#rows + 1] = row end
  return rows
end

local function column_stats(tbl)
  local count = #tbl.colspecs
  local longest_text, longest_word = {}, {}
  for i = 1, count do longest_text[i], longest_word[i] = 0, 0 end
  for _, row in ipairs(cell_texts(tbl)) do
    for i, cell in ipairs(row.cells) do
      if i <= count then
        local text = pandoc.utils.stringify(cell.contents)
        longest_text[i] = math.max(longest_text[i], utf8.len(text) or #text)
        for word in text:gmatch("%S+") do
          longest_word[i] = math.max(longest_word[i], utf8.len(word) or #word)
        end
      end
    end
  end
  return longest_text, longest_word
end

local function normalise(weights)
  local total = 0
  for _, w in ipairs(weights) do total = total + w end
  local out = {}
  for i, w in ipairs(weights) do out[i] = w / total end
  return out
end

-- Relative widths: damped by square root so one long column does not starve
-- the others, and never narrower than its longest unbreakable word.
local function relative_widths(longest_text, longest_word)
  local weights = {}
  for i = 1, #longest_text do
    weights[i] = math.sqrt(math.max(longest_text[i], 1))
  end
  local shares = normalise(weights)
  for i = 1, #shares do
    local floor = math.max(MIN_COLUMN_SHARE, math.min(longest_word[i] / CHARS_PER_LINE, 0.5))
    shares[i] = math.max(shares[i], floor)
  end
  return normalise(shares)
end

local function fits_naturally(longest_text)
  local total = 0
  for _, n in ipairs(longest_text) do total = total + n + 3 end
  return total <= NATURAL_TABLE_MAX_CHARS
end

function Table(tbl)
  if #tbl.colspecs == 0 then return nil end
  local longest_text, longest_word = column_stats(tbl)
  if fits_naturally(longest_text) then
    return nil
  end
  local widths = relative_widths(longest_text, longest_word)
  local specs = {}
  for i, spec in ipairs(tbl.colspecs) do
    specs[i] = { spec[1], widths[i] }
  end
  tbl.colspecs = specs
  return tbl
end

-- Deep headings become bold paragraphs: LaTeX stops at \subparagraph.
function Header(el)
  if el.level > MAX_HEADING_LEVEL then
    return pandoc.Para({ pandoc.Strong(el.content) })
  end
  return nil
end

-- A paragraph holding only line breaks (a lone backslash in the source)
-- would typeset as an empty line with no text.
function Para(el)
  if #el.content == 0 then return {} end
  for _, inline in ipairs(el.content) do
    if inline.t ~= "LineBreak" and inline.t ~= "SoftBreak" and inline.t ~= "Space" then
      return nil
    end
  end
  return {}
end
