#!/bin/bash

# Generate PNG and SVG images from Mermaid diagram files
# Requires: mermaid-cli (mmdc)
# Install: npm install -g @mermaid-js/mermaid-cli

set -e

DIAGRAM_DIR="docs/diagrams"
OUTPUT_DIR="docs/diagrams/images"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Mermaid Diagram to Image Generator       ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""

# Check if mmdc is installed
if ! command -v mmdc &> /dev/null; then
    echo -e "${RED}Error: mermaid-cli (mmdc) is not installed${NC}"
    echo ""
    echo "Install it with:"
    echo "  npm install -g @mermaid-js/mermaid-cli"
    echo ""
    echo "Or with yarn:"
    echo "  yarn global add @mermaid-js/mermaid-cli"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Find all .mmd files
mmd_files=$(find "$DIAGRAM_DIR" -maxdepth 1 -name "*.mmd")

if [ -z "$mmd_files" ]; then
    echo -e "${RED}No .mmd files found in $DIAGRAM_DIR${NC}"
    exit 1
fi

echo -e "${GREEN}Found Mermaid diagram files:${NC}"
echo "$mmd_files" | while read file; do
    basename "$file"
done
echo ""

# Generate images
echo -e "${BLUE}Generating images...${NC}"
echo ""

total=0
success=0
failed=0

for mmd_file in $mmd_files; do
    filename=$(basename "$mmd_file" .mmd)
    total=$((total + 1))

    echo -e "${BLUE}Processing: ${filename}${NC}"

    # Generate PNG
    if mmdc -i "$mmd_file" -o "$OUTPUT_DIR/${filename}.png" -b transparent -w 1920 -H 1080; then
        echo -e "  ✓ PNG created: ${OUTPUT_DIR}/${filename}.png"
        success=$((success + 1))
    else
        echo -e "${RED}  ✗ Failed to create PNG${NC}"
        failed=$((failed + 1))
    fi

    # Generate SVG
    if mmdc -i "$mmd_file" -o "$OUTPUT_DIR/${filename}.svg" -b transparent; then
        echo -e "  ✓ SVG created: ${OUTPUT_DIR}/${filename}.svg"
    else
        echo -e "${RED}  ✗ Failed to create SVG${NC}"
    fi

    echo ""
done

# Summary
echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Generation Complete                       ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Total diagrams: ${total}${NC}"
echo -e "${GREEN}Successful: ${success}${NC}"
if [ $failed -gt 0 ]; then
    echo -e "${RED}Failed: ${failed}${NC}"
fi
echo ""
echo -e "${GREEN}Images saved to: ${OUTPUT_DIR}${NC}"
echo ""

# Create an index HTML to preview all diagrams
echo -e "${BLUE}Creating preview HTML...${NC}"
cat > "$OUTPUT_DIR/index.html" << 'EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Boddle Auth Gateway - Diagrams</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
            padding: 20px;
        }
        header {
            background: #2c3e50;
            color: white;
            padding: 30px;
            text-align: center;
            border-radius: 8px;
            margin-bottom: 30px;
        }
        h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        .subtitle {
            font-size: 1.2em;
            opacity: 0.9;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .diagram-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 30px;
        }
        .diagram-card {
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            overflow: hidden;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .diagram-card:hover {
            transform: translateY(-4px);
            box-shadow: 0 4px 16px rgba(0,0,0,0.15);
        }
        .diagram-header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
        }
        .diagram-header h2 {
            font-size: 1.4em;
            margin-bottom: 5px;
        }
        .diagram-category {
            font-size: 0.9em;
            opacity: 0.9;
        }
        .diagram-content {
            padding: 20px;
        }
        .diagram-content img {
            width: 100%;
            height: auto;
            border: 1px solid #e0e0e0;
            border-radius: 4px;
            cursor: pointer;
        }
        .button-group {
            display: flex;
            gap: 10px;
            margin-top: 15px;
        }
        .btn {
            flex: 1;
            padding: 10px;
            border: none;
            border-radius: 4px;
            font-size: 0.9em;
            cursor: pointer;
            text-decoration: none;
            text-align: center;
            transition: background 0.2s;
        }
        .btn-primary {
            background: #667eea;
            color: white;
        }
        .btn-primary:hover {
            background: #5568d3;
        }
        .btn-secondary {
            background: #e0e0e0;
            color: #333;
        }
        .btn-secondary:hover {
            background: #d0d0d0;
        }
        .modal {
            display: none;
            position: fixed;
            z-index: 1000;
            left: 0;
            top: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.9);
            padding: 40px;
        }
        .modal-content {
            max-width: 90%;
            max-height: 90%;
            margin: auto;
            display: block;
        }
        .close {
            position: absolute;
            top: 20px;
            right: 40px;
            color: white;
            font-size: 40px;
            font-weight: bold;
            cursor: pointer;
        }
        footer {
            text-align: center;
            margin-top: 50px;
            padding: 20px;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🔐 Boddle Authentication Gateway</h1>
            <p class="subtitle">System Architecture Diagrams</p>
        </header>

        <div class="diagram-grid" id="diagramGrid"></div>

        <footer>
            <p>&copy; 2024 Boddle Learning. Generated from Mermaid diagrams.</p>
        </footer>
    </div>

    <!-- Modal for full-size image -->
    <div id="imageModal" class="modal">
        <span class="close">&times;</span>
        <img class="modal-content" id="modalImage">
    </div>

    <script>
        const diagrams = [
            {
                name: "System Architecture",
                category: "Overview",
                filename: "system-architecture",
                description: "Complete system architecture with all components"
            },
            {
                name: "Deployment Architecture",
                category: "Deployment",
                filename: "deployment-architecture",
                description: "Kubernetes and AWS deployment configurations"
            },
            {
                name: "Database Architecture",
                category: "Database",
                filename: "database-architecture",
                description: "Database schema, ERD, and data flow"
            },
            {
                name: "Monitoring & Observability",
                category: "Operations",
                filename: "monitoring-observability",
                description: "Monitoring stack and observability tools"
            },
            {
                name: "Current System Flows",
                category: "Authentication",
                filename: "current-system-flows",
                description: "Current Rails authentication flows"
            },
            {
                name: "New System Flows",
                category: "Authentication",
                filename: "new-system-flows",
                description: "New Go Gateway authentication flows"
            }
        ];

        const grid = document.getElementById('diagramGrid');
        const modal = document.getElementById('imageModal');
        const modalImg = document.getElementById('modalImage');
        const closeBtn = document.getElementsByClassName('close')[0];

        diagrams.forEach(diagram => {
            const card = document.createElement('div');
            card.className = 'diagram-card';
            card.innerHTML = `
                <div class="diagram-header">
                    <h2>${diagram.name}</h2>
                    <div class="diagram-category">${diagram.category}</div>
                </div>
                <div class="diagram-content">
                    <img src="${diagram.filename}.png"
                         alt="${diagram.name}"
                         onclick="openModal('${diagram.filename}.png')"
                         onerror="this.src='${diagram.filename}.svg'">
                    <div class="button-group">
                        <a href="${diagram.filename}.png" download class="btn btn-primary">
                            ⬇️ Download PNG
                        </a>
                        <a href="${diagram.filename}.svg" download class="btn btn-secondary">
                            ⬇️ Download SVG
                        </a>
                    </div>
                </div>
            `;
            grid.appendChild(card);
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
EOF

echo -e "${GREEN}✓ Preview HTML created: ${OUTPUT_DIR}/index.html${NC}"
echo ""
echo -e "${BLUE}To view the diagrams:${NC}"
echo -e "  open ${OUTPUT_DIR}/index.html"
echo ""
