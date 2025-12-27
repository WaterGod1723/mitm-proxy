-- html_inject.lua - HTML内容注入脚本
-- 可用全局变量：
--   url: string - 响应对应的请求URL
--   status: number - 响应状态码
--   headers: table - 响应头表（只读）

-- 示例1: 注入自定义CSS样式
function inject_custom_css()
    return [[
        <style>
        /* Lua MITM Proxy 注入的自定义样式 */
        body {
            border: 2px solid #4CAF50 !important;
            margin: 10px !important;
            padding: 10px !important;
        }
        
        .lua-proxy-badge {
            position: fixed;
            top: 10px;
            right: 10px;
            background-color: #4CAF50;
            color: white;
            padding: 5px 10px;
            border-radius: 5px;
            font-family: Arial, sans-serif;
            font-size: 12px;
            z-index: 9999;
        }
        </style>
    ]]
end

-- 示例2: 注入自定义JavaScript代码
function inject_custom_js()
    return [[
        <script>
        // Lua MITM Proxy 注入的自定义JavaScript
        console.log('页面被Lua MITM Proxy处理');
        
        // 创建一个代理标记
        const badge = document.createElement('div');
        badge.className = 'lua-proxy-badge';
        badge.textContent = 'Powered by Lua MITM Proxy';
        document.body.appendChild(badge);
        
        // 监听点击事件
        document.addEventListener('click', function(e) {
            console.log('点击事件:', e.target);
        });
        </script>
    ]]
end

-- 示例3: 根据URL注入不同内容
function inject_url_specific_content()
    if string.find(url, "google.com") then
        return [[
            <div style="position:fixed; bottom:10px; left:10px; background:yellow; padding:10px;">
                这是Google页面的特殊注入内容
            </div>
        ]]
    elseif string.find(url, "github.com") then
        return [[
            <div style="position:fixed; bottom:10px; left:10px; background:blue; color:white; padding:10px;">
                这是GitHub页面的特殊注入内容
            </div>
        ]]
    end
    return ""
end

-- 主HTML注入函数
function inject_html()
    print("Injecting HTML into: " .. url)
    
    -- 组合所有注入内容
    local css = inject_custom_css()
    local js = inject_custom_js()
    local specific = inject_url_specific_content()
    
    return css .. js .. specific
end