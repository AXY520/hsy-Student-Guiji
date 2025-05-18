class MarkerInfo extends HTMLElement {
    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.markers = [];
        this.currentMarker = null;
    }

    connectedCallback() {
        this.render();
        this.setupStyles();
    }

    setupStyles() {
        const style = document.createElement('style');
        style.textContent = `
            :host {
                position: fixed;
                top: 20px;
                right: 20px;
                z-index: 1000;
                font-family: 'PingFang SC', 'Microsoft YaHei', sans-serif;
            }

            .info-container {
                background: rgba(255, 255, 255, 0.7);
                backdrop-filter: blur(10px);
                -webkit-backdrop-filter: blur(10px);
                border-radius: 15px;
                padding: 20px;
                box-shadow: 0 8px 32px 0 rgba(31, 38, 135, 0.37);
                border: 1px solid rgba(255, 255, 255, 0.18);
                min-width: 250px;
                color: #333;
            }

            .stats {
                margin-bottom: 15px;
                padding-bottom: 15px;
                border-bottom: 1px solid rgba(0, 0, 0, 0.1);
            }

            .current-marker {
                display: none;
            }

            .current-marker.active {
                display: block;
            }

            h3 {
                margin: 0 0 10px 0;
                font-size: 16px;
                color: #1a73e8;
            }

            p {
                margin: 5px 0;
                font-size: 14px;
                line-height: 1.4;
            }

            .coordinates {
                color: #666;
                font-size: 13px;
                margin-top: 8px;
            }

            .image-count {
                color: #1a73e8;
                font-weight: 500;
            }

            @media (max-width: 768px) {
                :host {
                    top: 10px;
                    right: 10px;
                }

                .info-container {
                    padding: 15px;
                    min-width: 200px;
                }
            }
        `;
        this.shadowRoot.appendChild(style);
    }

    render() {
        this.shadowRoot.innerHTML = `
            <div class="info-container">
                <div class="stats">
                    <h3>点位统计</h3>
                    <p>总点位数：<span id="total-count">0</span></p>
                    <p>总图片数：<span id="total-images">0</span></p>
                </div>
                <div class="current-marker">
                    <h3>当前点位</h3>
                    <p class="description"></p>
                    <p class="coordinates"></p>
                    <p class="image-count"></p>
                </div>
            </div>
        `;
    }

    updateStats(markers) {
        this.markers = markers;
        const totalImages = markers.reduce((sum, marker) => sum + (marker.images ? marker.images.length : 0), 0);
        
        const totalCount = this.shadowRoot.querySelector('#total-count');
        const totalImagesElem = this.shadowRoot.querySelector('#total-images');
        
        totalCount.textContent = markers.length;
        totalImagesElem.textContent = totalImages;
    }

    updateCurrentMarker(marker) {
        this.currentMarker = marker;
        const currentMarkerDiv = this.shadowRoot.querySelector('.current-marker');
        
        if (marker) {
            currentMarkerDiv.classList.add('active');
            currentMarkerDiv.querySelector('.description').textContent = marker.description || '暂无描述';
            currentMarkerDiv.querySelector('.coordinates').textContent = 
                `坐标：${marker.latitude.toFixed(6)}, ${marker.longitude.toFixed(6)}`;
            currentMarkerDiv.querySelector('.image-count').textContent = 
                `图片数量：${marker.images ? marker.images.length : 0}`;
        } else {
            currentMarkerDiv.classList.remove('active');
        }
    }
}

customElements.define('marker-info', MarkerInfo); 