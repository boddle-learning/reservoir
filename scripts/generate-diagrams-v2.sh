#!/bin/bash

# Extract and generate images from Mermaid code blocks in markdown files
set -e

DIAGRAM_DIR="docs/diagrams"
OUTPUT_DIR="docs/diagrams/images"
TEMP_DIR=".tmp_mermaid"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Mermaid Diagram Image Generator          ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""

# Create directories
mkdir -p "$OUTPUT_DIR"
mkdir -p "$TEMP_DIR"

# Function to extract mermaid blocks from a file
extract_mermaid_blocks() {
    local file="$1"
    local output_prefix="$2"
    local count=0
    local in_block=false
    local current_block=""

    while IFS= read -r line; do
        if [[ "$line" == '```mermaid' ]]; then
            in_block=true
            current_block=""
        elif [[ "$line" == '```' ]] && [ "$in_block" = true ]; then
            in_block=false
            count=$((count + 1))
            echo "$current_block" > "${TEMP_DIR}/${output_prefix}_${count}.mmd"
        elif [ "$in_block" = true ]; then
            current_block="${current_block}${line}"$'\n'
        fi
    done < "$file"

    echo $count
}

total_diagrams=0
successful=0
failed=0

# Process all .mmd files (which are actually markdown)
for mmd_file in "$DIAGRAM_DIR"/*.mmd; do
    [ -f "$mmd_file" ] || continue

    filename=$(basename "$mmd_file" .mmd)
    echo -e "${BLUE}Processing: ${filename}${NC}"

    # Extract mermaid blocks
    block_count=$(extract_mermaid_blocks "$mmd_file" "$filename")

    if [ "$block_count" -eq 0 ]; then
        echo -e "${YELLOW}  ⚠ No Mermaid blocks found${NC}"
        continue
    fi

    echo -e "  Found ${block_count} diagram(s)"

    # Generate images for each block
    for i in $(seq 1 $block_count); do
        temp_file="${TEMP_DIR}/${filename}_${i}.mmd"

        if [ ! -f "$temp_file" ]; then
            continue
        fi

        total_diagrams=$((total_diagrams + 1))

        output_name="${filename}-${i}"

        # Generate PNG
        if mmdc -i "$temp_file" -o "$OUTPUT_DIR/${output_name}.png" -b transparent -w 1920 2>/dev/null; then
            echo -e "  ${GREEN}✓${NC} ${output_name}.png"
            successful=$((successful + 1))
        else
            echo -e "  ${RED}✗${NC} ${output_name}.png (failed)"
            failed=$((failed + 1))
        fi

        # Generate SVG
        if mmdc -i "$temp_file" -o "$OUTPUT_DIR/${output_name}.svg" -b transparent 2>/dev/null; then
            echo -e "  ${GREEN}✓${NC} ${output_name}.svg"
        else
            echo -e "  ${RED}✗${NC} ${output_name}.svg (failed)"
        fi
    done

    echo ""
done

# Clean up temp files
rm -rf "$TEMP_DIR"

# Summary
echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Generation Complete                       ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Total diagrams processed: ${total_diagrams}${NC}"
echo -e "${GREEN}Successful: ${successful}${NC}"
if [ $failed -gt 0 ]; then
    echo -e "${RED}Failed: ${failed}${NC}"
fi
echo ""
echo -e "${GREEN}Images saved to: ${OUTPUT_DIR}${NC}"
echo ""

# Create HTML preview
cat > "$OUTPUT_DIR/index.html" << 'HTMLEOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Boddle Auth Gateway - Architecture Diagrams</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 20px;
            min-height: 100vh;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        header {
            background: rgba(255, 255, 255, 0.95);
            color: #2c3e50;
            padding: 40px;
            text-align: center;
            border-radius: 12px;
            margin-bottom: 30px;
            box-shadow: 0 8px 32px rgba(0,0,0,0.1);
        }
        h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .subtitle {
            font-size: 1.2em;
            color: #666;
        }
        .stats {
            display: flex;
            justify-content: center;
            gap: 30px;
            margin-top: 20px;
        }
        .stat {
            text-align: center;
        }
        .stat-number {
            font-size: 2em;
            font-weight: bold;
            color: #667eea;
        }
        .stat-label {
            font-size: 0.9em;
            color: #666;
        }
        .diagram-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(450px, 1fr));
            gap: 25px;
            margin-bottom: 30px;
        }
        .diagram-card {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 12px;
            overflow: hidden;
            transition: transform 0.3s, box-shadow 0.3s;
            box-shadow: 0 4px 16px rgba(0,0,0,0.1);
        }
        .diagram-card:hover {
            transform: translateY(-5px);
            box-shadow: 0 8px 32px rgba(0,0,0,0.2);
        }
        .diagram-header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
        }
        .diagram-header h2 {
            font-size: 1.3em;
            margin-bottom: 5px;
        }
        .diagram-number {
            font-size: 0.85em;
            opacity: 0.9;
        }
        .diagram-content {
            padding: 20px;
        }
        .diagram-content img {
            width: 100%;
            height: auto;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            cursor: pointer;
            transition: border-color 0.3s;
        }
        .diagram-content img:hover {
            border-color: #667eea;
        }
        .button-group {
            display: flex;
            gap: 10px;
            margin-top: 15px;
        }
        .btn {
            flex: 1;
            padding: 12px;
            border: none;
            border-radius: 8px;
            font-size: 0.9em;
            font-weight: 600;
            cursor: pointer;
            text-decoration: none;
            text-align: center;
            transition: all 0.3s;
        }
        .btn-primary {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }
        .btn-primary:hover {
            transform: scale(1.05);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }
        .btn-secondary {
            background: #f0f0f0;
            color: #333;
        }
        .btn-secondary:hover {
            background: #e0e0e0;
        }
        .modal {
            display: none;
            position: fixed;
            z-index: 1000;
            left: 0;
            top: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.95);
            padding: 40px;
            animation: fadeIn 0.3s;
        }
        @keyframes fadeIn {
            from { opacity: 0; }
            to { opacity: 1; }
        }
        .modal-content {
            max-width: 95%;
            max-height: 95%;
            margin: auto;
            display: block;
            animation: zoomIn 0.3s;
        }
        @keyframes zoomIn {
            from { transform: scale(0.8); }
            to { transform: scale(1); }
        }
        .close {
            position: absolute;
            top: 20px;
            right: 40px;
            color: white;
            font-size: 50px;
            font-weight: bold;
            cursor: pointer;
            transition: color 0.3s;
        }
        .close:hover {
            color: #667eea;
        }
        footer {
            text-align: center;
            margin-top: 50px;
            padding: 30px;
            background: rgba(255, 255, 255, 0.95);
            border-radius: 12px;
            color: #666;
        }
        .category-section {
            margin-bottom: 40px;
        }
        .category-header {
            background: rgba(255, 255, 255, 0.95);
            padding: 20px 30px;
            border-radius: 12px;
            margin-bottom: 20px;
        }
        .category-header h2 {
            color: #2c3e50;
            font-size: 1.8em;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🔐 Boddle Authentication Gateway</h1>
            <p class="subtitle">System Architecture Diagrams</p>
            <div class="stats">
                <div class="stat">
                    <div class="stat-number" id="totalDiagrams">0</div>
                    <div class="stat-label">Total Diagrams</div>
                </div>
                <div class="stat">
                    <div class="stat-number">4</div>
                    <div class="stat-label">Categories</div>
                </div>
                <div class="stat">
                    <div class="stat-number">PNG + SVG</div>
                    <div class="stat-label">Formats</div>
                </div>
            </div>
        </header>

        <div id="diagramsContainer"></div>

        <footer>
            <p>&copy; 2024 Boddle Learning. Generated automatically from Mermaid diagrams.</p>
            <p style="margin-top: 10px; font-size: 0.9em;">Click any diagram to view full size • ESC to close</p>
        </footer>
    </div>

    <div id="imageModal" class="modal">
        <span class="close">&times;</span>
        <img class="modal-content" id="modalImage">
    </div>

    <script>
        const categories = {
            'system-architecture': {
                name: 'System Architecture',
                description: 'Complete system overview with all components'
            },
            'deployment-architecture': {
                name: 'Deployment Architecture',
                description: 'Kubernetes, AWS, and Docker Compose configurations'
            },
            'database-architecture': {
                name: 'Database Architecture',
                description: 'ERD, sharding, connection pooling, and backup strategies'
            },
            'monitoring-observability': {
                name: 'Monitoring & Observability',
                description: 'Monitoring stack, dashboards, alerts, and tracing'
            }
        };

        const container = document.getElementById('diagramsContainer');
        const modal = document.getElementById('imageModal');
        const modalImg = document.getElementById('modalImage');
        const closeBtn = document.getElementsByClassName('close')[0];

        // Find all PNG files in the directory
        fetch('.')
            .then(resp => resp.text())
            .then(html => {
                const parser = new DOMParser();
                const doc = parser.parseFromString(html, 'text/html');
                const links = Array.from(doc.querySelectorAll('a'))
                    .map(a => a.getAttribute('href'))
                    .filter(href => href && href.endsWith('.png'));

                // Group by category
                const grouped = {};
                links.forEach(link => {
                    const match = link.match(/(.+)-(\d+)\.png$/);
                    if (match) {
                        const category = match[1];
                        if (!grouped[category]) {
                            grouped[category] = [];
                        }
                        grouped[category].push(link);
                    }
                });

                let totalCount = 0;

                // Render each category
                Object.entries(grouped).forEach(([category, images]) => {
                    const categoryInfo = categories[category] || { name: category, description: '' };

                    const section = document.createElement('div');
                    section.className = 'category-section';

                    section.innerHTML = `
                        <div class="category-header">
                            <h2>${categoryInfo.name}</h2>
                            <p style="color: #666; margin-top: 5px;">${categoryInfo.description}</p>
                        </div>
                        <div class="diagram-grid" id="grid-${category}"></div>
                    `;

                    container.appendChild(section);

                    const grid = document.getElementById(`grid-${category}`);

                    images.sort().forEach((img, idx) => {
                        totalCount++;
                        const baseName = img.replace('.png', '');
                        const card = document.createElement('div');
                        card.className = 'diagram-card';
                        card.innerHTML = `
                            <div class="diagram-header">
                                <h2>${categoryInfo.name}</h2>
                                <div class="diagram-number">Diagram ${idx + 1} of ${images.length}</div>
                            </div>
                            <div class="diagram-content">
                                <img src="${img}" alt="${categoryInfo.name} ${idx + 1}" onclick="openModal('${img}')">
                                <div class="button-group">
                                    <a href="${img}" download class="btn btn-primary">
                                        ⬇️ Download PNG
                                    </a>
                                    <a href="${baseName}.svg" download class="btn btn-secondary">
                                        ⬇️ Download SVG
                                    </a>
                                </div>
                            </div>
                        `;
                        grid.appendChild(card);
                    });
                });

                document.getElementById('totalDiagrams').textContent = totalCount;
            });

        function openModal(src) {
            modal.style.display = 'block';
            modalImg.src = src;
        }

        closeBtn.onclick = function() {
            modal.style.display = 'none';
        }

        modal.onclick = function(event) {
            if (event.target === modal) {
                modal.style.display = 'none';
            }
        }

        document.addEventListener('keydown', function(event) {
            if (event.key === 'Escape') {
                modal.style.display = 'none';
            }
        });
    </script>
</body>
</html>
HTMLEOF

echo -e "${GREEN}✓ Preview HTML created: ${OUTPUT_DIR}/index.html${NC}"
echo ""
echo -e "${BLUE}To view the diagrams:${NC}"
echo -e "  open ${OUTPUT_DIR}/index.html"
echo ""
