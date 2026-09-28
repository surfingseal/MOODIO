package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/surfingseal/MOODIO/internal/config"
	"github.com/surfingseal/MOODIO/internal/model"
	"github.com/surfingseal/MOODIO/internal/service"
	"golang.org/x/oauth2"
)

type pendingPlaylist struct {
	req       model.PlaylistRequest
	createdAt time.Time
}

// Handler HTTP 웹 요청 처리기
type Handler struct {
	cfg              *config.Config
	gemini           *service.GeminiService
	mu               sync.RWMutex
	pendingPlaylists map[string]pendingPlaylist
}

// New 새로운 핸들러 인스턴스를 생성합니다.
func New(cfg *config.Config, gemini *service.GeminiService) *Handler {
	return &Handler{
		cfg:              cfg,
		gemini:           gemini,
		pendingPlaylists: make(map[string]pendingPlaylist),
	}
}

// RegisterRoutes 라우터에 엔드포인트를 등록합니다.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.HandleHome)
	mux.HandleFunc("/api/analyze", h.HandleAnalyzeImage)
	mux.HandleFunc("/api/create-flow", h.HandleCreateFlow)
	mux.HandleFunc("/auth/google/login", h.HandleGoogleLogin)
	mux.HandleFunc("/auth/google/callback", h.HandleGoogleCallback)
}

func generateRandomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) savePendingPlaylist(state string, req model.PlaylistRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingPlaylists[state] = pendingPlaylist{
		req:       req,
		createdAt: time.Now(),
	}
}

func (h *Handler) getAndRemovePendingPlaylist(state string) (model.PlaylistRequest, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	item, exists := h.pendingPlaylists[state]
	if exists {
		delete(h.pendingPlaylists, state)
		return item.req, true
	}
	return model.PlaylistRequest{}, false
}

// HandleHome 메인 홈 화면
func (h *Handler) HandleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := `<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MOODIO - 사진으로 완성하는 AI 유튜브 뮤직 플레이리스트</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700&family=Noto+Sans+KR:wght@400;500;700&display=swap" rel="stylesheet">
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: 'Plus Jakarta Sans', 'Noto Sans KR', -apple-system, sans-serif;
      min-height: 100vh;
      background: radial-gradient(circle at 50% 0%, #1e1b4b, #0f172a 50%, #030712);
      color: #f8fafc;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 32px 16px;
    }
    .container {
      width: 100%;
      max-width: 620px;
    }
    .card {
      background: rgba(30, 41, 59, 0.75);
      backdrop-filter: blur(20px);
      border: 1px solid rgba(255, 255, 255, 0.12);
      border-radius: 28px;
      padding: 40px 32px;
      box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.6);
    }
    .header { text-align: center; margin-bottom: 28px; }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 6px 14px;
      background: rgba(239, 68, 68, 0.15);
      color: #f87171;
      border: 1px solid rgba(239, 68, 68, 0.3);
      border-radius: 9999px;
      font-size: 13px;
      font-weight: 600;
      margin-bottom: 16px;
    }
    h1 {
      font-size: 26px;
      font-weight: 700;
      line-height: 1.3;
      margin-bottom: 8px;
      background: linear-gradient(135deg, #ffffff 40%, #cbd5e1);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
    }
    p.desc { color: #94a3b8; font-size: 14px; line-height: 1.5; }
    
    /* Upload Dropzone */
    .dropzone {
      border: 2px dashed rgba(255, 255, 255, 0.2);
      border-radius: 20px;
      padding: 32px 20px;
      text-align: center;
      cursor: pointer;
      transition: all 0.2s ease;
      background: rgba(15, 23, 42, 0.4);
      position: relative;
      margin-bottom: 20px;
    }
    .dropzone:hover, .dropzone.dragover {
      border-color: #ef4444;
      background: rgba(239, 68, 68, 0.05);
    }
    .dropzone input { display: none; }
    .dropzone-icon {
      font-size: 36px;
      margin-bottom: 8px;
    }
    .dropzone-text {
      font-size: 14px;
      font-weight: 600;
      color: #e2e8f0;
      margin-bottom: 4px;
    }
    .dropzone-sub { font-size: 12px; color: #64748b; }
    
    .preview-container {
      display: none;
      margin-bottom: 20px;
      text-align: center;
      position: relative;
    }
    .preview-img {
      max-height: 220px;
      max-width: 100%;
      border-radius: 16px;
      border: 1px solid rgba(255, 255, 255, 0.1);
      object-fit: cover;
    }
    .remove-btn {
      position: absolute;
      top: 10px;
      right: calc(50% - 110px);
      background: rgba(0, 0, 0, 0.7);
      color: #fff;
      border: none;
      border-radius: 50%;
      width: 28px;
      height: 28px;
      cursor: pointer;
      font-size: 14px;
    }

    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 10px;
      width: 100%;
      padding: 15px 24px;
      background: linear-gradient(135deg, #ef4444, #dc2626);
      color: #ffffff;
      border: none;
      font-size: 15px;
      font-weight: 600;
      border-radius: 14px;
      cursor: pointer;
      transition: all 0.2s ease;
      box-shadow: 0 8px 20px -4px rgba(239, 68, 68, 0.5);
    }
    .btn:hover:not(:disabled) {
      transform: translateY(-2px);
      box-shadow: 0 12px 24px -4px rgba(239, 68, 68, 0.6);
    }
    .btn:disabled {
      opacity: 0.5;
      cursor: not-allowed;
      transform: none;
      box-shadow: none;
    }

    /* Loading Spinner */
    .loading {
      display: none;
      text-align: center;
      padding: 24px 0;
    }
    .spinner {
      width: 36px;
      height: 36px;
      border: 3px solid rgba(239, 68, 68, 0.2);
      border-top-color: #ef4444;
      border-radius: 50%;
      animation: spin 0.8s linear infinite;
      margin: 0 auto 12px;
    }
    @keyframes spin { to { transform: rotate(360deg); } }

    /* Result Section */
    .result-section {
      display: none;
      margin-top: 24px;
      animation: fadeIn 0.3s ease;
    }
    @keyframes fadeIn { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
    .result-box {
      background: rgba(15, 23, 42, 0.6);
      border: 1px solid rgba(255, 255, 255, 0.08);
      border-radius: 18px;
      padding: 20px;
      margin-bottom: 20px;
    }
    .mood-tag {
      display: inline-block;
      font-size: 12px;
      color: #38bdf8;
      background: rgba(56, 189, 248, 0.1);
      border: 1px solid rgba(56, 189, 248, 0.2);
      padding: 4px 10px;
      border-radius: 6px;
      margin-bottom: 10px;
      font-weight: 600;
    }
    .playlist-title {
      font-size: 18px;
      font-weight: 700;
      color: #fff;
      margin-bottom: 6px;
    }
    .playlist-desc {
      font-size: 13px;
      color: #94a3b8;
      margin-bottom: 16px;
    }
    .track-list { list-style: none; }
    .track-item {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 10px 12px;
      background: rgba(255, 255, 255, 0.03);
      border-radius: 10px;
      margin-bottom: 8px;
      font-size: 14px;
    }
    .track-num {
      color: #ef4444;
      font-weight: 700;
      font-size: 13px;
      width: 20px;
    }
    .track-thumb, .track-thumb-placeholder {
      width: 44px;
      height: 33px;
      border-radius: 6px;
      flex-shrink: 0;
    }
    .track-thumb {
      object-fit: cover;
      background: #1e293b;
      box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
    }
    .track-thumb-placeholder {
      display: flex;
      align-items: center;
      justify-content: center;
      background: rgba(255, 255, 255, 0.06);
      border: 1px solid rgba(255, 255, 255, 0.1);
      font-size: 14px;
      color: #94a3b8;
    }
    .track-info { flex: 1; min-width: 0; text-align: left; }
    .track-title { font-weight: 600; color: #f1f5f9; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .track-artist { font-size: 12px; color: #94a3b8; }
  </style>
</head>
<body>
  <div class="container">
    <div class="card">
      <div class="header">
        <div class="badge">✨ GEMINI 3.5 FLASH-LITE × YOUTUBE MUSIC</div>
        <h1>사진으로 만드는 맞춤 플레이리스트</h1>
        <p class="desc">여행지나 일상 사진을 올리면 Gemini AI가 분위기를 읽고 어울리는 음악을 자동 선곡하여 유튜브 보관함에 담아드립니다.</p>
      </div>

      <!-- Dropzone -->
      <div class="dropzone" id="dropzone" onclick="document.getElementById('fileInput').click()">
        <input type="file" id="fileInput" accept="image/jpeg,image/png,image/webp">
        <div class="dropzone-icon">📸</div>
        <div class="dropzone-text">사진을 클릭하거나 드래그하여 업로드하세요</div>
        <div class="dropzone-sub">JPG, PNG, WebP 지원 (최대 10MB)</div>
      </div>

      <!-- Preview -->
      <div class="preview-container" id="previewContainer">
        <img id="previewImg" class="preview-img" alt="미리보기">
        <button class="remove-btn" id="removeBtn" title="사진 제거">✕</button>
      </div>

      <!-- Action Button -->
      <button class="btn" id="analyzeBtn" disabled>
        ✨ 사진 분위기 분석 및 선곡하기
      </button>

      <!-- Loading -->
      <div class="loading" id="loading">
        <div class="spinner"></div>
        <p style="font-size: 14px; color: #e2e8f0; font-weight: 600;">Gemini가 사진의 무드를 감상하고 있습니다...</p>
        <p style="font-size: 12px; color: #64748b; margin-top: 4px;">분위기에 딱 맞는 곡들을 엄선하는 중입니다</p>
      </div>

      <!-- Result Section -->
      <div class="result-section" id="resultSection">
        <div class="result-box">
          <div class="mood-tag" id="moodSummary">감성 요약</div>
          <div class="playlist-title" id="playlistTitle">플레이리스트 제목</div>
          <div class="playlist-desc" id="playlistDesc">설명</div>
          <ul class="track-list" id="trackList"></ul>
        </div>
        <button class="btn" id="createPlaylistBtn">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z"/>
          </svg>
          내 YouTube 보관함에 이 플레이리스트 생성하기
        </button>
      </div>
    </div>
  </div>

  <script>
    let selectedFile = null;
    let analysisResult = null;

    const fileInput = document.getElementById('fileInput');
    const dropzone = document.getElementById('dropzone');
    const previewContainer = document.getElementById('previewContainer');
    const previewImg = document.getElementById('previewImg');
    const removeBtn = document.getElementById('removeBtn');
    const analyzeBtn = document.getElementById('analyzeBtn');
    const loading = document.getElementById('loading');
    const resultSection = document.getElementById('resultSection');
    const createPlaylistBtn = document.getElementById('createPlaylistBtn');

    // Drag & Drop
    ['dragenter', 'dragover'].forEach(name => {
      dropzone.addEventListener(name, (e) => { e.preventDefault(); dropzone.classList.add('dragover'); });
    });
    ['dragleave', 'drop'].forEach(name => {
      dropzone.addEventListener(name, (e) => { e.preventDefault(); dropzone.classList.remove('dragover'); });
    });
    dropzone.addEventListener('drop', (e) => {
      if (e.dataTransfer.files && e.dataTransfer.files[0]) {
        handleFileSelect(e.dataTransfer.files[0]);
      }
    });
    fileInput.addEventListener('change', (e) => {
      if (e.target.files && e.target.files[0]) {
        handleFileSelect(e.target.files[0]);
      }
    });

    function handleFileSelect(file) {
      if (!file.type.startsWith('image/')) {
        alert('이미지 파일만 업로드할 수 있습니다.');
        return;
      }
      selectedFile = file;
      const reader = new FileReader();
      reader.onload = (e) => {
        previewImg.src = e.target.result;
        previewContainer.style.display = 'block';
        dropzone.style.display = 'none';
        analyzeBtn.disabled = false;
        resultSection.style.display = 'none';
      };
      reader.readAsDataURL(file);
    }

    removeBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      selectedFile = null;
      fileInput.value = '';
      previewContainer.style.display = 'none';
      dropzone.style.display = 'block';
      analyzeBtn.disabled = true;
      resultSection.style.display = 'none';
    });

    // 이미지 고속 압축 함수 (최대 1024px, JPEG 85% 품질로 변환하여 업로드 및 AI 분석 속도 극대화)
    function resizeImage(file, maxDimension = 1024, quality = 0.85) {
      return new Promise((resolve) => {
        const reader = new FileReader();
        reader.onload = (e) => {
          const img = new Image();
          img.onload = () => {
            let width = img.width;
            let height = img.height;

            if (width > height) {
              if (width > maxDimension) {
                height = Math.round((height * maxDimension) / width);
                width = maxDimension;
              }
            } else {
              if (height > maxDimension) {
                width = Math.round((width * maxDimension) / height);
                height = maxDimension;
              }
            }

            const canvas = document.createElement('canvas');
            canvas.width = width;
            canvas.height = height;
            const ctx = canvas.getContext('2d');
            ctx.drawImage(img, 0, 0, width, height);

            canvas.toBlob((blob) => {
              resolve(blob || file);
            }, 'image/jpeg', quality);
          };
          img.src = e.target.result;
        };
        reader.readAsDataURL(file);
      });
    }

    // AI Analyze
    analyzeBtn.addEventListener('click', async () => {
      if (!selectedFile) return;

      analyzeBtn.style.display = 'none';
      loading.style.display = 'block';
      resultSection.style.display = 'none';

      try {
        // 고용량 사진을 브라우저에서 1024px로 고속 압축(약 150KB)하여 네트워크 및 AI 처리 시간 단축
        const compressedBlob = await resizeImage(selectedFile);
        const formData = new FormData();
        formData.append('image', compressedBlob, 'photo.jpg');

        const res = await fetch('/api/analyze', { method: 'POST', body: formData });
        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || '분석에 실패했습니다.');
        }

        analysisResult = await res.json();
        renderResult(analysisResult);
      } catch (err) {
        alert('오류: ' + err.message);
        analyzeBtn.style.display = 'block';
      } finally {
        loading.style.display = 'none';
      }
    });

    function renderResult(data) {
      document.getElementById('moodSummary').textContent = '🌿 ' + data.mood_summary;
      document.getElementById('playlistTitle').textContent = data.playlist_title;
      document.getElementById('playlistDesc').textContent = data.playlist_description;

      const list = document.getElementById('trackList');
      list.innerHTML = '';
      data.tracks.forEach((track, idx) => {
        const li = document.createElement('li');
        li.className = 'track-item';
        li.innerHTML = '<span class="track-num">' + (idx + 1) + '</span><div class="track-thumb-placeholder">🎵</div><div class="track-info"><div class="track-title">' + track.title + '</div><div class="track-artist">' + track.artist + '</div></div>';
        list.appendChild(li);
      });

      resultSection.style.display = 'block';
    }

    // Create YouTube Playlist
    createPlaylistBtn.addEventListener('click', async () => {
      if (!analysisResult) return;

      createPlaylistBtn.disabled = true;
      createPlaylistBtn.textContent = '구글 로그인 화면으로 이동 중...';

      try {
        const res = await fetch('/api/create-flow', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            title: analysisResult.playlist_title,
            description: analysisResult.playlist_description + ' (Mood: ' + analysisResult.mood_summary + ')',
            privacy_status: 'public',
            tracks: analysisResult.tracks,
          })
        });

        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText);
        }

        const data = await res.json();
        window.location.href = data.redirect_url;
      } catch (err) {
        alert('플레이리스트 생성 준비 실패: ' + err.message);
        createPlaylistBtn.disabled = false;
        createPlaylistBtn.textContent = '내 YouTube 보관함에 이 플레이리스트 생성하기';
      }
    });
  </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// HandleAnalyzeImage 사진 업로드 및 Gemini 분위기 분석 처리
func (h *Handler) HandleAnalyzeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.gemini == nil {
		http.Error(w, "서버에 GEMINI_API_KEY가 설정되지 않아 이미지 분석을 진행할 수 없습니다. .env에 GEMINI_API_KEY를 등록해주세요.", http.StatusServiceUnavailable)
		return
	}

	// 10MB 크기 제한
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "업로드 파일 크기가 너무 큽니다 (최대 10MB)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "이미지 파일이 전달되지 않았습니다", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "이미지 읽기 실패", http.StatusInternalServerError)
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(imageBytes)
	}

	result, err := h.gemini.AnalyzeMoodFromImage(r.Context(), imageBytes, mimeType)
	if err != nil {
		log.Printf("❌ Gemini 분석 실패: %v", err)
		http.Error(w, fmt.Sprintf("사진 분석 오류: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

// HandleCreateFlow 분석된 곡 목록으로 YouTube OAuth 로그인 플로우 시작
func (h *Handler) HandleCreateFlow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.PlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "요청 바디 파싱 실패", http.StatusBadRequest)
		return
	}

	if len(req.Tracks) == 0 {
		http.Error(w, "선곡된 트랙 목록이 비어있습니다", http.StatusBadRequest)
		return
	}

	state := generateRandomState()
	h.savePendingPlaylist(state, req)

	oauthConfig := h.cfg.OAuth2Config()
	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"redirect_url": authURL,
	})
}

// HandleGoogleLogin 고정 기본 로그인 (사진 없이 바로 연동할 경우)
func (h *Handler) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if h.cfg.GoogleClientID == "" || h.cfg.GoogleClientSecret == "" {
		http.Error(w, "서버에 GOOGLE_CLIENT_ID 또는 GOOGLE_CLIENT_SECRET이 설정되지 않았습니다.", http.StatusServiceUnavailable)
		return
	}

	oauthConfig := h.cfg.OAuth2Config()
	url := oauthConfig.AuthCodeURL(h.cfg.OAuthState, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleGoogleCallback OAuth 인증 완료 후 토큰 교환 및 플레이리스트 등록 수행
func (h *Handler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "인증 코드가 전달되지 않았습니다.", http.StatusBadRequest)
		return
	}

	var playlistReq model.PlaylistRequest
	if pending, exists := h.getAndRemovePendingPlaylist(state); exists {
		playlistReq = pending
	} else if subtle.ConstantTimeCompare([]byte(state), []byte(h.cfg.OAuthState)) == 1 {
		// 고정 기본 트랙 fallback
		playlistReq = model.PlaylistRequest{
			Title:         "비 내리는 늦은 오후의 산책 🌧️",
			Description:   "사진 분위기 맞춤 자동 선곡 플레이리스트 (MOODIO)",
			PrivacyStatus: "public",
			Tracks: []model.Track{
				{Artist: "아이유", Title: "밤편지"},
				{Artist: "헤이즈", Title: "비도 오고 그래서"},
				{Artist: "폴킴", Title: "모든 날 모든 순간"},
			},
		}
	} else {
		http.Error(w, "잘못된 State 토큰입니다. (CSRF 검증 실패)", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	oauthConfig := h.cfg.OAuth2Config()

	// 1. 코드를 Access Token으로 교환
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		log.Printf("❌ OAuth 토큰 교환 실패: %v", err)
		http.Error(w, fmt.Sprintf("토큰 발급 실패: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. YouTube 서비스 인스턴스 초기화
	ytService, err := service.NewYouTubeService(ctx, oauthConfig, token)
	if err != nil {
		log.Printf("❌ YouTube 서비스 생성 실패: %v", err)
		http.Error(w, fmt.Sprintf("YouTube API 클라이언트 초기화 실패: %v", err), http.StatusInternalServerError)
		return
	}

	// 3. 사용자가 분석한 맞춤 곡으로 플레이리스트 생성 및 등록
	playlistURL, err := ytService.GeneratePlaylistWithTracks(ctx, playlistReq)
	if err != nil {
		log.Printf("❌ 플레이리스트 처리 실패: %v", err)
		http.Error(w, fmt.Sprintf("플레이리스트 생성 실패: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("🎉 완성된 플레이리스트: %s", playlistURL)
	http.Redirect(w, r, playlistURL, http.StatusSeeOther)
}
