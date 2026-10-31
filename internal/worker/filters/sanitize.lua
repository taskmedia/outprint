-- First filter. Makes the document safe to hand to the Pandoc LaTeX writer:
-- raw HTML is reduced to line breaks, links are limited to safe targets, and
-- anything that could carry TeX into the output makes the job fail.
--
-- Failures are raised as "renderer-filter:<kind>: <fixed text>". The text
-- never contains document content. The worker maps the kind to an error.

local FAIL_PREFIX = "renderer-filter:"
local BUNDLE_IMAGE_DIR = "images/"
local SAFE_LINK_SCHEMES = { http = true, https = true, mailto = true }

local function fail(kind, text)
  error(FAIL_PREFIX .. kind .. ": " .. text, 0)
end

local function is_tex_format(format)
  local f = format:lower()
  return f == "latex" or f == "tex"
end

local function is_line_break_tag(text)
  return text:lower():match("^<br%s*/?>$") ~= nil
end

local function check_raw_format(el)
  if is_tex_format(el.format) then
    fail("raw_tex", "raw TeX node reached the filter stage")
  end
  if el.format ~= "html" then
    fail("raw_format", "raw node of an unexpected format")
  end
end

-- Raw HTML other than <br> is dropped: the LaTeX writer would drop the tags
-- anyway, and nothing from it may reach the output. Text between inline tags
-- is ordinary text and is kept.
function RawInline(el)
  check_raw_format(el)
  if is_line_break_tag(el.text) then
    return pandoc.LineBreak()
  end
  return {}
end

function RawBlock(el)
  check_raw_format(el)
  return {}
end

function Math()
  fail("math", "math node found")
end

local function has_scheme(target)
  return target:match("^[%a][%w+.-]*:") ~= nil
end

local function is_bundle_image(src)
  if not src:find("^" .. BUNDLE_IMAGE_DIR .. "[%w._-]+$") then
    return false
  end
  if src:find("%.%.") or src:sub(#BUNDLE_IMAGE_DIR + 1, #BUNDLE_IMAGE_DIR + 1) == "." then
    return false
  end
  local file = io.open(src, "rb")
  if not file then
    return false
  end
  file:close()
  return true
end

function Image(el)
  if has_scheme(el.src) or not is_bundle_image(el.src) then
    fail("image_path", "image does not refer to a file of the job bundle")
  end
end

-- Links keep their text; only web, mail and in-document targets stay links.
function Link(el)
  local target = el.target
  if target:sub(1, 1) == "#" then
    return nil
  end
  local scheme = target:match("^([%a][%w+.-]*):")
  if scheme and SAFE_LINK_SCHEMES[scheme:lower()] then
    return nil
  end
  return el.content
end
